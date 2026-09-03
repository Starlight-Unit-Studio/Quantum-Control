package main

import (
	"context"
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
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/control"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("quantum-control stopped", "error", err)
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
		_, err := config.LoadControl()
		if err != nil {
			return err
		}
		fmt.Println("configuration valid")
		return nil
	case "serve":
		return serve()
	default:
		return fmt.Errorf("unknown command %q; expected serve, version, or check-config", command)
	}
}

func serve() error {
	cfg, err := config.LoadControl()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	brokerClient := broker.NewClient(cfg.BrokerSocket, cfg.BrokerToken, cfg.BrokerTimeout)
	controlServer := control.NewServer(brokerClient, cfg, logger)
	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           controlServer,
		ReadHeaderTimeout: cfg.HeaderTimeout,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      cfg.BrokerTimeout + 5*time.Second,
		IdleTimeout:       cfg.IdleTimeout,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		logger.Info("quantum-control listening", "address", cfg.Listen, "version", buildinfo.Version)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
