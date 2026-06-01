package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DeadManSwitch polls a Kubernetes Lease and reports lease validity to the
// control-state machine. The canary CronJob renews the lease; if it stops
// (e.g. cluster scheduling is broken), the lease expires and the monkey
// suspends. Observations are level-triggered: every tick reports the current
// lease state and the state machine decides what (if anything) changes.
type DeadManSwitch struct {
	client         kubernetes.Interface
	leaseName      string
	leaseNamespace string
	autoResume     bool

	mu        sync.Mutex
	lastRenew time.Time
	leaseTTL  time.Duration
}

// NewDeadManSwitch creates a new DeadManSwitch.
func NewDeadManSwitch(client kubernetes.Interface, leaseName, leaseNamespace string, autoResume bool) *DeadManSwitch {
	return &DeadManSwitch{
		client:         client,
		leaseName:      leaseName,
		leaseNamespace: leaseNamespace,
		autoResume:     autoResume,
	}
}

// AutoResume returns whether auto-resume is enabled.
func (d *DeadManSwitch) AutoResume() bool {
	return d.autoResume
}

// Status returns the last observed lease renew time and its computed expiry.
// Both are zero until a valid lease has been seen.
func (d *DeadManSwitch) Status() (lastRenew, expiresAt time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.lastRenew.IsZero() || d.leaseTTL == 0 {
		return time.Time{}, time.Time{}
	}
	return d.lastRenew, d.lastRenew.Add(d.leaseTTL)
}

// watchLoop polls the lease and reports validity via onValid/onExpired until
// ctx is cancelled.
func (d *DeadManSwitch) watchLoop(ctx context.Context, onValid, onExpired func()) {
	// Check immediately on startup.
	d.checkLease(ctx, onValid, onExpired)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.checkLease(ctx, onValid, onExpired)
		}
	}
}

func (d *DeadManSwitch) checkLease(ctx context.Context, onValid, onExpired func()) {
	lease, err := d.client.CoordinationV1().Leases(d.leaseNamespace).Get(ctx, d.leaseName, metav1.GetOptions{})
	if err != nil {
		slog.Warn("dead man's switch: failed to get lease", "error", err, "name", d.leaseName, "namespace", d.leaseNamespace)
		onExpired()
		return
	}
	if lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil || *lease.Spec.LeaseDurationSeconds <= 0 {
		slog.Warn("dead man's switch: lease missing renewTime or leaseDurationSeconds")
		onExpired()
		return
	}

	renewTime := lease.Spec.RenewTime.Time
	leaseTTL := time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second

	d.mu.Lock()
	d.lastRenew = renewTime
	d.leaseTTL = leaseTTL
	d.mu.Unlock()

	if time.Now().After(renewTime.Add(leaseTTL)) {
		onExpired()
		return
	}

	onValid()
}
