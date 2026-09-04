package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/broker"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/buildinfo"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/config"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/servicecontrol"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/systemprobe"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("qcored stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "serve"
	if len(args) > 0 {
		command = args[0]
	}
	switch command {
	case "version", "--version", "-version":
		fmt.Println(buildinfo.Version)
		return nil
	case "check-config":
		cfg, err := config.LoadBroker()
		if err != nil {
			return err
		}
		if _, _, err := loadMutationSecurity(cfg); err != nil {
			return err
		}
		fmt.Println("configuration and privileged security state valid")
		return nil
	case "catalog":
		registry := broker.NewRegistry(systemprobe.Native{})
		if err := registry.EnableServiceMutations(servicecontrol.Native{}, servicecontrol.HTTPHealth{}, servicecontrol.DefaultPolicy(), 30*time.Second, 250*time.Millisecond); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(registry.Catalog())
	case "serve":
		return serve()
	default:
		return fmt.Errorf("unknown command %q; expected serve, version, check-config, or catalog", command)
	}
}

func serve() error {
	cfg, err := config.LoadBroker()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	boundary, policy, err := loadMutationSecurity(cfg)
	if err != nil {
		return fmt.Errorf("initialize privileged security state: %w", err)
	}
	registry := broker.NewRegistry(systemprobe.Native{})
	if err := registry.EnableServiceMutations(servicecontrol.Native{}, servicecontrol.HTTPHealth{}, policy, cfg.TransactionTimeout, cfg.ServicePollInterval); err != nil {
		return fmt.Errorf("initialize service mutation registry: %w", err)
	}
	listener, err := broker.ListenUnix(cfg.SocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{
		Handler:           broker.NewServerWithSecurity(registry, cfg.BrokerToken, cfg.RequestBodyLimit, logger, boundary).Handler(),
		ReadHeaderTimeout: cfg.HeaderTimeout,
		ReadTimeout:       cfg.TransactionTimeout + 10*time.Second,
		WriteTimeout:      cfg.TransactionTimeout + 15*time.Second,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		logger.Info("qcored listening", "socket", cfg.SocketPath, "version", buildinfo.Version)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		return <-errCh
	}
}

func loadMutationSecurity(cfg config.Broker) (broker.SecurityBoundary, servicecontrol.Policy, error) {
	policy, err := servicecontrol.LoadPolicy(cfg.ServicePolicyFile)
	if err != nil {
		return broker.SecurityBoundary{}, servicecontrol.Policy{}, err
	}
	grants, err := security.OpenGrantStore(cfg.GrantPath, cfg.GrantTTL)
	if err != nil {
		return broker.SecurityBoundary{}, servicecontrol.Policy{}, fmt.Errorf("open broker confirmation store: %w", err)
	}
	var authenticator security.Authenticator
	if cfg.ActorFile != "" {
		registry, err := security.LoadActorRegistry(cfg.ActorFile)
		if err != nil {
			return broker.SecurityBoundary{}, servicecontrol.Policy{}, fmt.Errorf("load broker actor registry: %w", err)
		}
		authenticator = registry
	}
	return broker.SecurityBoundary{Actors: authenticator, Grants: grants}, policy, nil
}
