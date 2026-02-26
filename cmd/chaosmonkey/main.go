package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/postfinance/chaosmonkey/pkg/profile"
)

func getKubeConfig() (*rest.Config, error) {
	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
}

func main() {
	// Subcommand routing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "renew-lease":
			runRenewLease()
			return
		}
	}

	interval := flag.Duration("interval", time.Minute, "calc interval")
	profilesPath := flag.String("profiles", "", "path to profiles YAML file")
	defaultProfile := flag.String("defaultProfile", "default", "profile name for pods without label")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	dryRun := flag.Bool("dry-run", false, "log evictions without performing them")
	listenAddr := flag.String("listen", ":8080", "HTTP listen address")
	enableDashboard := flag.Bool("dashboard", true, "enable web dashboard")

	timezone := flag.String("timezone", "UTC", "timezone for dashboard display (e.g. Europe/Zurich)")

	// Dead man's switch flags
	dmsEnabled := flag.Bool("dms-enabled", false, "enable dead man's switch")
	dmsAutoResume := flag.Bool("dms-auto-resume", false, "auto-resume when lease is renewed after expiry")

	flag.Parse()

	var level slog.Level
	switch strings.ToLower(*logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		slog.Error("invalid log level", "level", *logLevel)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	profiles, err := loadProfiles(*profilesPath)
	if err != nil {
		slog.Error("failed to load profiles", "error", err)
		os.Exit(1)
	}
	if _, ok := profiles[*defaultProfile]; !ok {
		slog.Error("default profile not found", "profile", *defaultProfile)
		os.Exit(1)
	}
	for name := range profiles {
		slog.Info("loaded profile", "name", name)
	}

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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	loc, err := time.LoadLocation(*timezone)
	if err != nil {
		slog.Error("invalid timezone", "timezone", *timezone, "error", err)
		os.Exit(1)
	}

	m := NewMonkey(client, profiles, *defaultProfile, *interval, *dryRun, loc)

	if *dmsEnabled {
		dms := NewDeadManSwitch(client, defaultLeaseName, defaultLeaseNamespace, *dmsAutoResume)
		m.SetDeadManSwitch(dms)
	}

	mux := http.NewServeMux()
	m.RegisterAPI(mux)
	if *enableDashboard {
		m.RegisterDashboard(mux, ctx)
	}

	srv := &http.Server{Addr: *listenAddr, Handler: mux}
	go func() {
		slog.Info("starting HTTP server", "addr", *listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server failed", "error", err)
		}
	}()

	m.Start(ctx)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown failed", "error", err)
	}
	slog.Info("shutting down")
}

func loadProfiles(path string) (map[string]*profile.KillProfile, error) {
	if path != "" {
		return profile.Load(path)
	}
	return map[string]*profile.KillProfile{
		"default": {MaxAge: 100 * 8760 * time.Hour},
	}, nil
}
