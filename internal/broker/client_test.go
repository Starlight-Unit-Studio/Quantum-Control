package broker

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
)

func TestClientPreservesSemanticPlanAndExecutionResponses(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "qcored.sock")
	listener, err := ListenUnix(socketPath)
	if err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}

	const token = "01234567890123456789012345678901"
	registry := NewRegistry(&fakeProbe{})
	server := &http.Server{
		Handler: NewServer(
			registry,
			token,
			1<<20,
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		).Handler(),
		ReadHeaderTimeout: 2 * time.Second,
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		_ = listener.Close()
		<-serveDone
	})

	client := NewClient(socketPath, token, 2*time.Second)
	if err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}

	plan, err := client.Plan(context.Background(), protocol.OperationRequest{Action: "shell.exec"})
	if err != nil {
		t.Fatalf("Plan returned transport error for semantic rejection: %v", err)
	}
	if plan.Valid || plan.Error == nil || plan.Error.Code != "unknown_action" {
		t.Fatalf("unexpected plan: %#v", plan)
	}

	response, err := client.Execute(context.Background(), protocol.OperationRequest{Action: "shell.exec"})
	if err != nil {
		t.Fatalf("Execute returned transport error for semantic rejection: %v", err)
	}
	if response.Status != "rejected" || response.Error == nil || response.Error.Code != "unknown_action" {
		t.Fatalf("unexpected response: %#v", response)
	}
}
