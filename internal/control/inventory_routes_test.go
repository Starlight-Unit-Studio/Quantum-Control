package control

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/inventory"
)

type fakeInventoryScanner struct {
	snapshot inventory.Snapshot
	component inventory.Component
	found bool
	err error
}

func (f fakeInventoryScanner) Snapshot(context.Context) (inventory.Snapshot, error) {
	return f.snapshot, f.err
}

func (f fakeInventoryScanner) Component(context.Context, string) (inventory.Component, bool, error) {
	return f.component, f.found, f.err
}

func TestComponentInventoryRoutes(t *testing.T) {
	observed := time.Unix(10, 0).UTC()
	component := inventory.Component{
		ID: "quantum-runtime", Name: "Quantum Runtime", Category: "ai-runtime",
		Version: "0.2.0-alpha.2", Ownership: inventory.OwnershipManaged, Health: inventory.HealthHealthy,
		Services: []string{"quantum-runtime.service"}, Listeners: []inventory.Listener{}, Roots: []string{"/etc/quantum-runtime"},
		Capabilities: []string{"local-inference"}, Evidence: []inventory.Evidence{}, ObservedAt: observed, Warnings: []string{},
	}
	fake := fakeInventoryScanner{
		snapshot: inventory.Snapshot{Schema: inventory.SchemaVersion, ObservedAt: observed, Components: []inventory.Component{component}},
		component: component,
		found: true,
	}
	old := defaultInventoryScanner
	defaultInventoryScanner = fake
	t.Cleanup(func() { defaultInventoryScanner = old })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(NewServer(&fakeBroker{}, testConfig(""), logger))
	defer server.Close()

	response, err := http.Get(server.URL + "/v1/components")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected list status: %d", response.StatusCode)
	}
	var snapshot inventory.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Schema != inventory.SchemaVersion || len(snapshot.Components) != 1 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}

	response, err = http.Get(server.URL + "/v1/components/quantum-runtime")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected detail status: %d", response.StatusCode)
	}
}

func TestComponentInventoryRequiresAuth(t *testing.T) {
	old := defaultInventoryScanner
	defaultInventoryScanner = fakeInventoryScanner{snapshot: inventory.Snapshot{Schema: inventory.SchemaVersion}}
	t.Cleanup(func() { defaultInventoryScanner = old })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(NewServer(&fakeBroker{}, testConfig(testAPIToken), logger))
	defer server.Close()
	response, err := http.Get(server.URL + "/v1/components")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("inventory endpoint bypassed auth: %d", response.StatusCode)
	}
}

func TestUnknownComponentReturnsNotFound(t *testing.T) {
	old := defaultInventoryScanner
	defaultInventoryScanner = fakeInventoryScanner{found: false}
	t.Cleanup(func() { defaultInventoryScanner = old })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(NewServer(&fakeBroker{}, testConfig(""), logger))
	defer server.Close()
	response, err := http.Get(server.URL + "/v1/components/not-defined")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown component returned %d", response.StatusCode)
	}
}
