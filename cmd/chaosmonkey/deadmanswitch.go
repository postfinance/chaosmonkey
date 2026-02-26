package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/VictoriaMetrics/metrics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DeadManSwitch watches a Kubernetes Lease and triggers suspension when it expires.
type DeadManSwitch struct {
	client         kubernetes.Interface
	leaseName      string
	leaseNamespace string
	autoResume     bool

	mu          sync.Mutex
	enabled     bool
	expired     bool
	lastRenew   time.Time
	leaseTTL    time.Duration
	triggeredAt time.Time
}

// NewDeadManSwitch creates a new DeadManSwitch.
func NewDeadManSwitch(client kubernetes.Interface, leaseName, leaseNamespace string, autoResume bool) *DeadManSwitch {
	dms := &DeadManSwitch{
		client:         client,
		leaseName:      leaseName,
		leaseNamespace: leaseNamespace,
		autoResume:     autoResume,
		enabled:        true,
		expired:        true, // start expired until lease is positively verified
		triggeredAt:    time.Now(),
	}

	metrics.NewGauge("chaosmonkey_dms_expired", func() float64 {
		if dms.IsExpired() {
			return 1
		}
		return 0
	})

	return dms
}

// IsExpired returns whether the dead man's switch is currently triggered.
func (d *DeadManSwitch) IsExpired() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.expired
}

// Status returns the current DMS state.
func (d *DeadManSwitch) Status() (enabled, expired bool, lastRenew time.Time, expiresAt time.Time, triggeredAt time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	enabled = d.enabled
	expired = d.expired
	lastRenew = d.lastRenew
	triggeredAt = d.triggeredAt
	if !d.lastRenew.IsZero() && d.leaseTTL > 0 {
		expiresAt = d.lastRenew.Add(d.leaseTTL)
	}
	return
}

// AutoResume returns whether auto-resume is enabled.
func (d *DeadManSwitch) AutoResume() bool {
	return d.autoResume
}

// watchLoop polls the lease and calls onExpire/onResume callbacks.
func (d *DeadManSwitch) watchLoop(ctx context.Context, onExpire func(), onResume func()) {
	// Check immediately on startup
	d.checkLease(ctx, onExpire, onResume)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.checkLease(ctx, onExpire, onResume)
		}
	}
}

func (d *DeadManSwitch) checkLease(ctx context.Context, onExpire func(), onResume func()) {
	lease, err := d.client.CoordinationV1().Leases(d.leaseNamespace).Get(ctx, d.leaseName, metav1.GetOptions{})
	if err != nil {
		slog.Warn("dead man's switch: failed to get lease", "error", err, "name", d.leaseName, "namespace", d.leaseNamespace)
		// Treat missing/errored lease as expired
		d.handleExpiry(onExpire)
		return
	}

	if lease.Spec.RenewTime == nil {
		slog.Warn("dead man's switch: lease has no renewTime")
		d.handleExpiry(onExpire)
		return
	}
	if lease.Spec.LeaseDurationSeconds == nil || *lease.Spec.LeaseDurationSeconds <= 0 {
		slog.Warn("dead man's switch: lease has no valid leaseDurationSeconds")
		d.handleExpiry(onExpire)
		return
	}

	renewTime := lease.Spec.RenewTime.Time
	leaseTTL := time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
	expiresAt := renewTime.Add(leaseTTL)
	now := time.Now()

	d.mu.Lock()
	d.lastRenew = renewTime
	d.leaseTTL = leaseTTL
	wasExpired := d.expired
	d.mu.Unlock()

	if now.After(expiresAt) {
		d.handleExpiry(onExpire)
	} else if wasExpired {
		// Lease is valid again
		d.handleResume(onResume)
	}
}

func (d *DeadManSwitch) handleExpiry(onExpire func()) {
	d.mu.Lock()
	alreadyExpired := d.expired
	if !alreadyExpired {
		d.expired = true
		d.triggeredAt = time.Now()
	}
	d.mu.Unlock()

	if !alreadyExpired {
		slog.Error("dead man's switch triggered: lease expired", "name", d.leaseName, "namespace", d.leaseNamespace)
		onExpire()
	}
}

func (d *DeadManSwitch) handleResume(onResume func()) {
	d.mu.Lock()
	wasExpired := d.expired
	if wasExpired && d.autoResume {
		d.expired = false
		d.triggeredAt = time.Time{}
	}
	d.mu.Unlock()

	if wasExpired && d.autoResume {
		slog.Info("dead man's switch: lease renewed, auto-resuming")
		onResume()
	}
}
