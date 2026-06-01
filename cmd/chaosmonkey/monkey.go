package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VictoriaMetrics/metrics"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/postfinance/chaosmonkey/pkg/profile"
)

const (
	labelExclusion = "postfinance.ch/chaos-monkey-exclusion"
	labelProfile   = "postfinance.ch/chaos-monkey-profile"

	// annotationStaticPod is set by the kubelet on mirror pods that represent
	// static pods. Static pods are managed by the kubelet, not the API server,
	// so evicting/deleting them has no effect — exclude them.
	annotationStaticPod = "kubernetes.io/config.hash"
)

var (
	metricExcluded     = metrics.NewCounter("chaosmonkey_pods_excluded_total")
	metricEvaluated    = metrics.NewCounter("chaosmonkey_pods_evaluated_total")
	metricCalcDuration = metrics.NewHistogram("chaosmonkey_calc_duration_seconds")
)

// Monkey holds all runtime state for the chaos monkey.
type Monkey struct {
	client         kubernetes.Interface
	profiles       map[string]*profile.KillProfile
	defaultProfile string
	interval       time.Duration
	dryRun         bool
	startTime      time.Time
	location       *time.Location

	sched    *schedule
	control  *controlState
	dms      *DeadManSwitch
	eventLog *suspendEventLog

	totalKilled atomic.Uint64
	totalErrors atomic.Uint64
}

// NewMonkey creates a new Monkey instance.
func NewMonkey(client kubernetes.Interface, profiles map[string]*profile.KillProfile, defaultProfile string, interval time.Duration, dryRun bool, loc *time.Location) *Monkey {
	m := &Monkey{
		client:         client,
		profiles:       profiles,
		defaultProfile: defaultProfile,
		interval:       interval,
		dryRun:         dryRun,
		startTime:      time.Now(),
		location:       loc,
		sched:          newSchedule(),
		control:        &controlState{},
		eventLog:       newSuspendEventLog(10),
	}

	metrics.NewGauge("chaosmonkey_upcoming_kills", func() float64 {
		return float64(m.sched.upcomingCount(time.Now().Add(24 * time.Hour)))
	})

	metrics.NewGauge(fmt.Sprintf(`chaosmonkey_info{dry_run="%t",timezone=%q}`, dryRun, loc.String()), func() float64 {
		return 1
	})

	metrics.NewGauge("chaosmonkey_suspended", func() float64 {
		if m.control.isSuspended() {
			return 1
		}
		return 0
	})

	return m
}

// Suspend suspends evictions manually (e.g. via the /suspend endpoint). This
// always moves to MANUAL_RESUME_REQUIRED — only a human /resume clears it.
func (m *Monkey) Suspend(reason string) {
	if from, to, changed := m.control.manualSuspend(); changed {
		m.emitTransition(from, to, reason)
	}
}

// Resume resumes evictions manually (e.g. via the /resume endpoint).
func (m *Monkey) Resume(reason string) {
	if from, to, changed := m.control.manualResume(); changed {
		m.emitTransition(from, to, reason)
	}
}

// onLeaseValid is called by the dead man's switch on a healthy lease.
func (m *Monkey) onLeaseValid() {
	if from, to, changed := m.control.onLeaseValid(); changed {
		m.emitTransition(from, to, "dead man's switch")
	}
}

// onLeaseExpired is called by the dead man's switch on an expired/missing lease.
func (m *Monkey) onLeaseExpired() {
	if from, to, changed := m.control.onLeaseExpired(); changed {
		m.emitTransition(from, to, "dead man's switch")
	}
}

// emitTransition records metrics, the in-memory event log and a Kubernetes
// Event for a real state change.
func (m *Monkey) emitTransition(from, to controlStateValue, reason string) {
	if to == stateRunning {
		slog.Info("evictions resumed", "reason", reason, "from", from.String())
		metrics.GetOrCreateCounter(fmt.Sprintf(`chaosmonkey_resumptions_total{reason=%q}`, metricReason(reason))).Inc()
		m.eventLog.add("resumed", reason)
		m.emitSuspendEvent("EvictionsResumed", fmt.Sprintf("Evictions resumed: %s", reason))
		return
	}
	slog.Warn("evictions suspended", "reason", reason, "state", to.String())
	metrics.GetOrCreateCounter(fmt.Sprintf(`chaosmonkey_suspensions_total{reason=%q}`, metricReason(reason))).Inc()
	m.eventLog.add("suspended", reason)
	m.emitSuspendEvent("EvictionsSuspended", fmt.Sprintf("Evictions suspended: %s (%s)", reason, to))
}

// SetDeadManSwitch configures the dead man's switch. Evictions start suspended
// in WAITING_FOR_LEASE until the canary proves the lease is fresh.
func (m *Monkey) SetDeadManSwitch(dms *DeadManSwitch) {
	m.dms = dms
	m.control.init(stateWaitingForLease, dms.autoResume)
	slog.Info("dead man's switch enabled, waiting for valid lease", "auto_resume", dms.autoResume)
}

// Start runs the calc and kill loops, blocking until ctx is cancelled.
func (m *Monkey) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		m.calcLoop(ctx)
	}()
	go func() {
		defer wg.Done()
		m.killLoop(ctx)
	}()
	if m.dms != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.dms.watchLoop(ctx, m.onLeaseValid, m.onLeaseExpired)
		}()
	}
	wg.Wait()
}

func (m *Monkey) emitSuspendEvent(reason, note string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hostname := cmp.Or(os.Getenv("HOSTNAME"), "chaosmonkey")
	now := time.Now()
	event := &eventsv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "chaosmonkey-",
			Namespace:    cmp.Or(os.Getenv("POD_NAMESPACE"), "kube-chaosmonkey"),
		},
		Regarding: corev1.ObjectReference{
			Kind:       "Deployment",
			Name:       hostname,
			Namespace:  cmp.Or(os.Getenv("POD_NAMESPACE"), "kube-chaosmonkey"),
			APIVersion: "apps/v1",
		},
		Reason:              reason,
		Note:                note,
		Type:                "Warning",
		EventTime:           metav1.NewMicroTime(now),
		Action:              "SuspendControl",
		ReportingController: "chaosmonkey",
		ReportingInstance:   hostname,
	}
	if _, err := m.client.EventsV1().Events(event.Namespace).Create(ctx, event, metav1.CreateOptions{}); err != nil {
		slog.Warn("failed to create DMS event", "error", err)
	}
}

// RegisterAPI registers operational endpoints (healthz, metrics, suspend, resume).
func (m *Monkey) RegisterAPI(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		metrics.WritePrometheus(w, true)
	})
	mux.HandleFunc("/suspend", m.handleSuspend())
	mux.HandleFunc("/resume", m.handleResume())
}

func (m *Monkey) handleSuspend() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		m.Suspend("manual")
		_, _ = fmt.Fprintln(w, "suspended")
	}
}

func (m *Monkey) handleResume() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		m.Resume("manual")
		_, _ = fmt.Fprintln(w, "resumed")
	}
}

func (m *Monkey) profileNames() []string {
	names := make([]string, 0, len(m.profiles))
	for name := range m.profiles {
		names = append(names, name)
	}
	return names
}

// --- calc loop ---

func (m *Monkey) calcLoop(ctx context.Context) {
	for {
		m.calcTick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.interval):
		}
	}
}

func (m *Monkey) calcTick(ctx context.Context) {
	defer metricCalcDuration.UpdateDuration(time.Now())

	excludedNS := make(map[string]bool)
	nsList, err := m.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: labelExclusion + "=true",
	})
	if err != nil {
		slog.Error("failed to list namespaces", "error", err)
		return
	}
	for _, ns := range nsList.Items {
		excludedNS[ns.Name] = true
	}

	alreadyKnown := m.sched.knownUIDs()

	now := time.Now()
	var continueToken string
	var excluded int
	liveUIDs := make(map[string]struct{}, 1000)
	var newEntries []*podEntry

	for {
		pods, err := m.client.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			Limit:    500,
			Continue: continueToken,
		})
		if err != nil {
			slog.Error("failed to list pods", "error", err)
			return
		}

		for _, pod := range pods.Items {
			metricEvaluated.Inc()

			uid := string(pod.UID)

			liveUIDs[uid] = struct{}{}

			if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
				continue
			}

			if excludedNS[pod.Namespace] {
				excluded++
				metricExcluded.Inc()
				continue
			}

			// Static pods (mirror pods) are managed by the kubelet and cannot
			// be killed via the API server, so skip them.
			if _, ok := pod.Annotations[annotationStaticPod]; ok {
				excluded++
				metricExcluded.Inc()
				continue
			}

			if _, ok := alreadyKnown[uid]; ok {
				continue
			}

			profileName := m.defaultProfile
			p := m.profiles[profileName]
			if labelVal, ok := pod.Labels[labelProfile]; ok {
				if lp, found := m.profiles[labelVal]; found {
					profileName = labelVal
					p = lp
				} else {
					slog.Warn("unknown profile on pod, using default",
						"pod", pod.Namespace+"/"+pod.Name,
						"profile", labelVal,
					)
				}
			}

			deterministic := true
			killTime, err := p.KillTime(uid, pod.CreationTimestamp.Time, m.location)
			if err != nil {
				slog.Error("failed to compute deterministic kill time",
					"pod", pod.Namespace+"/"+pod.Name,
					"profile", profileName,
					"error", err,
				)
				continue
			}
			if killTime.IsZero() {
				continue
			}

			if killTime.Before(now) {
				killTime, err = p.KillTime(uid, now.Add(-p.MinAge), m.location)
				if err != nil {
					slog.Error("failed to compute catch-up kill time",
						"pod", pod.Namespace+"/"+pod.Name,
						"profile", profileName,
						"error", err,
					)
					continue
				}
				deterministic = false
				if killTime.IsZero() {
					continue
				}
			}

			newEntries = append(newEntries, &podEntry{
				Namespace:     pod.Namespace,
				Name:          pod.Name,
				UID:           uid,
				Profile:       profileName,
				KillMode:      p.KillMode,
				NodeName:      pod.Spec.NodeName,
				CreationTime:  pod.CreationTimestamp.Time,
				KillTime:      killTime,
				Deterministic: deterministic,
			})
		}

		continueToken = pods.Continue
		if continueToken == "" {
			break
		}
	}

	m.sched.reconcile(liveUIDs, newEntries, m.control.isSuspended(), m.profiles, m.location)
	slog.Info("calc tick", "excluded", excluded, "new", len(newEntries), "liveUIDs", len(liveUIDs))
}

// --- kill loop ---

func (m *Monkey) killLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.control.isSuspended() {
				continue
			}
			m.killTick(ctx)
		}
	}
}

func (m *Monkey) killTick(ctx context.Context) {
	overdue := m.sched.overdue(time.Now())

	for _, e := range overdue {
		podName := e.Namespace + "/" + e.Name
		mode := string(e.KillMode)
		if mode == "" {
			mode = "evict"
		}

		if m.dryRun {
			slog.Info("would kill pod", "pod", podName, "profile", e.Profile, "mode", mode)
			m.incKillMetric(e.Profile, mode, true)
			m.sched.markKilled(e.UID, "dry-run")
			continue
		}

		if err := m.killPod(ctx, e); err != nil {
			if apierrors.IsNotFound(err) {
				slog.Debug("pod gone, skipping", "pod", podName)
				m.sched.remove(e.UID)
				continue
			}
			m.totalErrors.Add(1)
			if apierrors.IsTooManyRequests(err) {
				metrics.GetOrCreateCounter(`chaosmonkey_kill_errors_total{reason="pdb_blocked"}`).Inc()
				slog.Warn("PDB blocked eviction", "pod", podName, "error", err)
				m.emitEvent(ctx, e, "ChaosMonkeyBlocked",
					fmt.Sprintf("Eviction blocked by PDB (profile: %s)", e.Profile))
			} else {
				metrics.GetOrCreateCounter(`chaosmonkey_kill_errors_total{reason="error"}`).Inc()
				slog.Error("kill failed", "pod", podName, "error", err)
			}
			m.sched.markKilled(e.UID, "error: "+err.Error())
			continue
		}

		age := time.Since(e.CreationTime).Truncate(time.Second)
		slog.Info("killed pod", "pod", podName, "profile", e.Profile, "mode", mode, "node", e.NodeName, "age", age)
		m.incKillMetric(e.Profile, mode, false)
		m.sched.markKilled(e.UID, mode)
		m.emitEvent(ctx, e, "ChaosMonkeyKilled",
			fmt.Sprintf("Killed by chaos monkey (profile: %s, mode: %s, age: %s)", e.Profile, mode, age))
	}
}

func (m *Monkey) incKillMetric(profileName, mode string, dryRun bool) {
	m.totalKilled.Add(1)
	metrics.GetOrCreateCounter(fmt.Sprintf(`chaosmonkey_pods_killed_total{profile=%q,mode=%q,dry_run="%t"}`, profileName, mode, dryRun)).Inc()
}

func (m *Monkey) killPod(ctx context.Context, e *podEntry) error {
	switch e.KillMode {
	case profile.KillModeDelete:
		return m.client.CoreV1().Pods(e.Namespace).Delete(ctx, e.Name, metav1.DeleteOptions{})
	case profile.KillModeForceDelete:
		var zero int64
		return m.client.CoreV1().Pods(e.Namespace).Delete(ctx, e.Name, metav1.DeleteOptions{
			GracePeriodSeconds: &zero,
		})
	default: // evict
		return m.client.CoreV1().Pods(e.Namespace).EvictV1(ctx, &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      e.Name,
				Namespace: e.Namespace,
			},
		})
	}
}

func (m *Monkey) emitEvent(ctx context.Context, e *podEntry, reason, note string) {
	now := time.Now()
	event := &eventsv1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "chaosmonkey-",
			Namespace:    e.Namespace,
		},
		Regarding: corev1.ObjectReference{
			Kind:       "Pod",
			Namespace:  e.Namespace,
			Name:       e.Name,
			UID:        types.UID(e.UID),
			APIVersion: "v1",
		},
		Reason:              reason,
		Note:                note,
		Type:                "Normal",
		EventTime:           metav1.NewMicroTime(now),
		Action:              "Evict",
		ReportingController: "chaosmonkey",
		ReportingInstance:   cmp.Or(os.Getenv("HOSTNAME"), "chaosmonkey"),
	}
	if _, err := m.client.EventsV1().Events(e.Namespace).Create(ctx, event, metav1.CreateOptions{}); err != nil {
		slog.Warn("failed to create event", "pod", e.Namespace+"/"+e.Name, "error", err)
	}
}
