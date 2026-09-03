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
		_, err := config.LoadBroker()
		if err != nil {
			return err
		}
		fmt.Println("configuration valid")
		return nil
	case "catalog":
		return json.NewEncoder(os.Stdout).Encode(broker.NewRegistry(systemprobe.Native{}).Catalog())
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

	listener, err := broker.ListenUnix(cfg.SocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()

	registry := broker.NewRegistry(systemprobe.Native{})
	server := &http.Server{
		Handler:           broker.NewServer(registry, cfg.BrokerToken, cfg.RequestBodyLimit, logger).Handler(),
		ReadHeaderTimeout: cfg.HeaderTimeout,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      20 * time.Second,
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
