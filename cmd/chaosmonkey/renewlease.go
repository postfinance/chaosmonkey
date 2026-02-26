package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultLeaseName      = "chaosmonkey-deadmanswitch"
	defaultLeaseDuration  = 120 // seconds
	defaultLeaseNamespace = "kube-chaosmonkey"
)

func runRenewLease() {
	fs := flag.NewFlagSet("renew-lease", flag.ExitOnError)
	leaseDurationSec := fs.Int("lease-duration", defaultLeaseDuration, "lease duration in seconds")
	fs.Parse(os.Args[2:])

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	restConfig, err := getKubeConfig()
	if err != nil {
		slog.Error("failed to get kubeconfig", "error", err)
		os.Exit(1)
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		slog.Error("failed to create client", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := renewLease(ctx, client, defaultLeaseNamespace, defaultLeaseName, int32(*leaseDurationSec)); err != nil {
		slog.Error("failed to renew lease", "error", err)
		os.Exit(1)
	}

	slog.Info("lease renewed", "name", defaultLeaseName, "namespace", defaultLeaseNamespace, "duration", fmt.Sprintf("%ds", *leaseDurationSec))
}

func renewLease(ctx context.Context, client kubernetes.Interface, namespace, name string, durationSec int32) error {
	now := metav1.NewMicroTime(time.Now())
	holderIdentity := "chaosmonkey-canary"

	lease, err := client.CoordinationV1().Leases(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		lease = &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &holderIdentity,
				LeaseDurationSeconds: &durationSec,
				RenewTime:            &now,
			},
		}
		_, err = client.CoordinationV1().Leases(namespace).Create(ctx, lease, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}

	lease.Spec.HolderIdentity = &holderIdentity
	lease.Spec.LeaseDurationSeconds = &durationSec
	lease.Spec.RenewTime = &now

	_, err = client.CoordinationV1().Leases(namespace).Update(ctx, lease, metav1.UpdateOptions{})
	return err
}
