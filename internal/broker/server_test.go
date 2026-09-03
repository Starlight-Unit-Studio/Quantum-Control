package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
)

func TestBrokerRequiresToken(t *testing.T) {
	registry := NewRegistry(&fakeProbe{})
	server := httptest.NewServer(NewServer(registry, "secret", 1<<20, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer server.Close()

	response, err := http.Get(server.URL + "/v1/operations")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", response.StatusCode)
	}
}

func TestBrokerExecutesTypedOperation(t *testing.T) {
	registry := NewRegistry(&fakeProbe{})
	server := httptest.NewServer(NewServer(registry, "secret", 1<<20, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler())
	defer server.Close()

	payload, _ := json.Marshal(protocol.OperationRequest{Action: "system.snapshot", Actor: "test"})
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL+"/v1/execute", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Quantum-Broker-Token", "secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("unexpected status %d: %s", response.StatusCode, data)
	}
	var result protocol.OperationResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" || result.Result["hostname"] != "test-node" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
