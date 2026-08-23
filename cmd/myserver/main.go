package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thotenn/myserver/internal/config"
	"github.com/thotenn/myserver/internal/discovery"
	"github.com/thotenn/myserver/internal/handlers"
	"github.com/thotenn/myserver/internal/scripts"
	"github.com/thotenn/myserver/internal/widgets"
	"go.uber.org/zap"
)

func main() {
	port := flag.Int("port", 3000, "HTTP server port")
	flag.Parse()

	logger := initLogger()
	defer logger.Sync()

	initConfig(logger)
	initAuth(logger)
	widgets.RegisterBuiltinWidgets()

	dockerDiscoverers := initDocker(logger)
	scriptMgr := initScripts(logger)

	watcher := startWatcher(logger, scriptMgr)
	if watcher != nil {
		defer watcher.Stop()
	}

	srv := startServer(logger, *port)
	waitForShutdown(logger, srv, dockerDiscoverers)
}

func initLogger() *zap.Logger {
	var logger *zap.Logger
	var err error
	if os.Getenv("DEBUG") == "true" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	return logger
}

func initConfig(logger *zap.Logger) {
	if err := config.EnsureConfigDir(); err != nil {
		logger.Fatal("failed to ensure config dir", zap.Error(err))
	}
	// A malformed prefix would silently serve the dashboard from the root and
	// emit URLs that do not resolve, so it is fatal at startup — the same
	// rule initAuth follows for a broken policy.
	if _, err := config.ParseBasePath(os.Getenv("HOMEPAGE_BASE_PATH")); err != nil {
		logger.Fatal("invalid HOMEPAGE_BASE_PATH; refusing to start", zap.Error(err))
	}
	if prefix := config.BasePath(); prefix != "" {
		logger.Info("serving under a base path", zap.String("basePath", prefix))
	}
	// The example files seed the ROOT config directory only: a client
	// dashboard's directory is written by the operator, and dropping the demo
	// dashboard into it would publish it to that client.
	if err := config.EnsureSkeleton(config.ConfigDir()); err != nil {
		logger.Warn("failed to seed the config directory", zap.Error(err))
	}

	set, errs := config.InitDashboards()
	for _, err := range errs {
		logger.Warn("ignoring a dashboard directory", zap.Error(err))
	}
	for _, d := range set.All() {
		d.Reload()
	}
	for _, d := range set.Clients() {
		logger.Info("serving a client dashboard",
			zap.String("dashboard", d.String()),
			zap.String("prefix", d.Prefix))
	}
	logger.Info("config cache initialised", zap.Int("dashboards", len(set.All())))
}

// initAuth reports every dashboard's authentication policy at startup and
// refuses to run with a broken one on the root dashboard.
//
// Startup is the only place that may be fatal. Once the process is serving,
// a bad edit to auth.yaml must never take a dashboard down or, far worse,
// open it: the watcher keeps the last known good policy instead.
//
// The root dashboard is fatal and a client is not, and the asymmetry is
// deliberate. A client with an unreadable policy is already failing closed —
// its own subtree answers 503 and says so in the log on every request — and
// taking the whole host down over one client's YAML would turn one broken
// dashboard into an outage for everybody else's.
func initAuth(logger *zap.Logger) {
	for _, d := range config.Dashboards().All() {
		initDashboardAuth(logger, d)
	}
}

func initDashboardAuth(logger *zap.Logger, d *config.Dashboard) {
	state := d.Auth()
	log := logger.With(zap.String("dashboard", d.String()),
		zap.String("file", config.AuthFile))

	if state.Err != nil {
		if d.IsRoot() {
			log.Fatal("authentication is configured but unusable; refusing to start",
				zap.Error(state.Err))
		}
		log.Error("authentication is configured but unusable; this dashboard "+
			"will answer 503 until it is fixed", zap.Error(state.Err))
		return
	}
	if !state.Required {
		if config.AuthRequiredEnv() {
			if d.IsRoot() {
				log.Fatal("HOMEPAGE_AUTH_REQUIRED=true but no allowlist is configured")
			}
			log.Error("HOMEPAGE_AUTH_REQUIRED=true but no allowlist is configured; " +
				"this dashboard will answer 503")
			return
		}
		if d.IsRoot() {
			log.Info("authentication disabled: dashboard is public")
		} else {
			// Client dashboards are normally gated — the allowlist is what
			// keeps one client out of another's. A public one is allowed, but
			// it is worth saying out loud that it is.
			log.Warn("no allowlist: this client dashboard is PUBLIC to anyone " +
				"who knows its URL")
		}
		return
	}

	log.Info("authentication enabled",
		zap.String("provider", state.Config.ProviderName()),
		zap.Int("allowedEmails", len(state.Config.Allowlist.Emails)),
		zap.Int("allowedDomains", len(state.Config.Allowlist.Domains)))
	if state.Config.UsesGeneratedSecret() {
		log.Warn("session.secret is not set: a random key was generated, " +
			"so every restart signs everybody out")
	}
}

func initDocker(logger *zap.Logger) []*discovery.DockerDiscoverer {
	var discoverers []*discovery.DockerDiscoverer
	// The root dashboard's: containers on the host belong to the host's own
	// dashboard and are never merged into a client's list.
	dockerConfigs, err := config.Dashboards().Root().Docker()
	if err != nil || len(dockerConfigs) == 0 {
		return discoverers
	}
	for name, cfg := range dockerConfigs {
		d, err := discovery.NewDockerDiscoverer(cfg)
		if err != nil {
			logger.Warn("failed to create docker discoverer", zap.String("server", name), zap.Error(err))
			continue
		}
		discoverers = append(discoverers, d)
	}
	handlers.SetDockerDiscoverers(discoverers)
	logger.Info("docker discovery initialized", zap.Int("servers", len(discoverers)))
	return discoverers
}

func initScripts(logger *zap.Logger) *scripts.Manager {
	if !config.ScriptsEnabled() {
		logger.Info("scripts feature disabled")
		return nil
	}

	settings, _ := config.Dashboards().Root().Settings()
	scriptDirs := []string{"/app/scripts"}
	defaultTimeout := 60
	maxTimeout := 3600
	maxConcurrent := 5
	if settings != nil && settings.ScriptSettings != nil {
		if len(settings.ScriptSettings.ScriptDirs) > 0 {
			scriptDirs = settings.ScriptSettings.ScriptDirs
		}
		if settings.ScriptSettings.MaxTimeout > 0 {
			maxTimeout = settings.ScriptSettings.MaxTimeout
		}
		if settings.ScriptSettings.DefaultTimeout > 0 {
			defaultTimeout = settings.ScriptSettings.DefaultTimeout
		}
		if settings.ScriptSettings.MaxConcurrent > 0 {
			maxConcurrent = settings.ScriptSettings.MaxConcurrent
		}
	}

	scriptMgr := scripts.NewManager(scriptDirs, defaultTimeout, maxTimeout, maxConcurrent)
	if err := registerScripts(scriptMgr, logger); err != nil {
		logger.Warn("failed to register scripts", zap.Error(err))
	}
	handlers.ScriptManager = scriptMgr
	logger.Info("scripts feature enabled", zap.Int("registered", len(scriptMgr.List())))
	return scriptMgr
}

func startWatcher(logger *zap.Logger, scriptMgr *scripts.Manager) *config.Watcher {
	watcher, err := config.NewWatcher(logger)
	if err != nil {
		logger.Warn("failed to create config watcher", zap.Error(err))
		return nil
	}
	// The watcher has already reloaded the dashboard it hands over; what is
	// left here is the process-wide state that hangs off the ROOT dashboard's
	// config, which is why none of it runs for a client.
	if err := watcher.Start(func(d *config.Dashboard) {
		logger.Info("config reloaded",
			zap.String("dashboard", d.String()),
			zap.String("hash", d.Hash()))
		logAuthReload(logger, d)
		if !d.IsRoot() {
			return
		}
		handlers.InvalidateProxyCache()
		handlers.InvalidateDockerClients()
		if scriptMgr != nil {
			if err := registerScripts(scriptMgr, logger); err != nil {
				logger.Warn("failed to reload scripts", zap.Error(err))
			}
		}
	}); err != nil {
		logger.Warn("failed to start config watcher", zap.Error(err))
		return nil
	}
	return watcher
}

func startServer(logger *zap.Logger, port int) *http.Server {
	handler := handlers.API(logger, port)
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		logger.Info("starting myserver",
			zap.String("addr", addr),
			zap.String("configDir", config.ConfigDir()),
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()
	return srv
}

func waitForShutdown(logger *zap.Logger, srv *http.Server, dockerDiscoverers []*discovery.DockerDiscoverer) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")
	for _, d := range dockerDiscoverers {
		if err := d.Close(); err != nil {
			logger.Warn("failed to close docker discoverer", zap.Error(err))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server forced to shutdown", zap.Error(err))
	}
	logger.Info("server stopped")
}

// registerScripts loads scripts.yaml and registers each script with the
// manager. On hot-reload it replaces the entire registry atomically.
func registerScripts(mgr *scripts.Manager, logger *zap.Logger) error {
	sf, err := config.Dashboards().Root().ScriptsFile()
	if err != nil {
		return fmt.Errorf("loading scripts.yaml: %w", err)
	}
	if sf == nil {
		return nil
	}
	var newScripts []*scripts.ScriptConfig
	for name, entry := range sf.Scripts {
		newScripts = append(newScripts, &scripts.ScriptConfig{
			Name:            name,
			Command:         entry.Command,
			Description:     entry.Description,
			Args:            entry.Args,
			Env:             entry.Env,
			Timeout:         entry.Timeout,
			RequireConfirm:  entry.RequireConfirm,
			Icon:            entry.Icon,
			AllowConcurrent: entry.AllowConcurrent,
			LogOutput:       entry.LogOutput,
		})
	}
	errs := mgr.ReplaceAll(newScripts)
	for _, e := range errs {
		logger.Warn("script registration failed", zap.Error(e))
	}
	if len(errs) > 0 {
		return fmt.Errorf("some scripts failed registration")
	}
	logger.Info("scripts reloaded", zap.Int("registered", len(newScripts)))
	return nil
}

// logAuthReload surfaces what happened to the authentication policy after a
// hot-reload. A policy that could not be re-read is the failure mode this
// feature exists to make loud.
func logAuthReload(logger *zap.Logger, d *config.Dashboard) {
	state := d.Auth()
	log := logger.With(zap.String("dashboard", d.String()))
	switch {
	case state.Lockdown:
		log.Error("auth policy unusable: serving 503 until it is fixed",
			zap.Error(state.Err), zap.String("file", config.AuthFile))
	case state.Degraded:
		log.Error("auth policy could not be re-read: keeping the last known good one",
			zap.Error(state.Err), zap.String("file", config.AuthFile))
	case state.Required:
		log.Info("auth policy reloaded",
			zap.Int("allowedEmails", len(state.Config.Allowlist.Emails)),
			zap.Int("allowedDomains", len(state.Config.Allowlist.Domains)))
	default:
		log.Info("auth policy reloaded: authentication is off, dashboard is public")
	}
}
