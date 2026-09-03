package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	Paths      []string                `json:"paths"`
	Files      map[string]string       `json:"files"`
	Commands   map[string]string       `json:"commands"`
	Services   map[string]ServiceState `json:"services"`
	Globs      map[string][]string     `json:"globs"`
	PathErrors []string                `json:"path_errors"`
}

type fixtureObserver struct {
	fixture fixture
	calls   []string
}

func loadFixture(t *testing.T, name string) *fixtureObserver {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var value fixture
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return &fixtureObserver{fixture: value}
}

func (f *fixtureObserver) PathExists(path string) (bool, error) {
	f.calls = append(f.calls, "path:"+path)
	for _, item := range f.fixture.PathErrors {
		if item == path {
			return false, errors.New("denied")
		}
	}
	for _, item := range f.fixture.Paths {
		if item == path {
			return true, nil
		}
	}
	if _, ok := f.fixture.Files[path]; ok {
		return true, nil
	}
	return false, nil
}

func (f *fixtureObserver) ReadFile(path string, _ int64) ([]byte, error) {
	f.calls = append(f.calls, "read:"+path)
	value, ok := f.fixture.Files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(value), nil
}

func (f *fixtureObserver) Command(_ context.Context, name string, args ...string) (string, error) {
	key := strings.TrimSpace(name + " " + strings.Join(args, " "))
	f.calls = append(f.calls, "command:"+key)
	value, ok := f.fixture.Commands[key]
	if !ok {
		return "", errors.New("unavailable")
	}
	return value, nil
}

func (f *fixtureObserver) Service(_ context.Context, unit string) (ServiceState, error) {
	f.calls = append(f.calls, "service:"+unit)
	if state, ok := f.fixture.Services[unit]; ok {
		return state, nil
	}
	return ServiceState{Unit: unit, LoadState: "not-found", ActiveState: "inactive", SubState: "dead"}, nil
}

func (f *fixtureObserver) Glob(pattern string) ([]string, error) {
	f.calls = append(f.calls, "glob:"+pattern)
	return append([]string{}, f.fixture.Globs[pattern]...), nil
}

func TestCleanFixtureIsDisabled(t *testing.T) {
	observer := loadFixture(t, "clean")
	scanner := NewScanner(observer)
	scanner.now = func() time.Time { return time.Unix(10, 0) }
	snapshot, err := scanner.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range snapshot.Components {
		if component.Ownership != OwnershipDisabled {
			t.Fatalf("%s ownership=%s", component.ID, component.Ownership)
		}
	}
}

func TestKeyHelpFixtureReportsExternalComponents(t *testing.T) {
	snapshot, err := NewScanner(loadFixture(t, "keyhelp")).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	keyhelp := findComponent(t, snapshot, "keyhelp")
	apache := findComponent(t, snapshot, "apache")
	if keyhelp.Ownership != OwnershipExternal || apache.Ownership != OwnershipExternal || apache.Health != HealthHealthy {
		t.Fatalf("unexpected inventory: keyhelp=%#v apache=%#v", keyhelp, apache)
	}
}

func TestStarlightFixtureRecognizesManagedRuntime(t *testing.T) {
	snapshot, err := NewScanner(loadFixture(t, "starlight")).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	runtime := findComponent(t, snapshot, "quantum-runtime")
	coreui := findComponent(t, snapshot, "ember-coreui")
	if runtime.Ownership != OwnershipManaged || runtime.Health != HealthHealthy {
		t.Fatalf("runtime not managed/healthy: %#v", runtime)
	}
	if coreui.Ownership != OwnershipExternal {
		t.Fatalf("CoreUI should remain externally owned: %#v", coreui)
	}
}

func TestStaleManagedMarkerFailsToUnknown(t *testing.T) {
	observer := &fixtureObserver{fixture: fixture{Paths: []string{"/etc/quantum-runtime/.managed.json"}}}
	component, ok, err := NewScanner(observer).Component(context.Background(), "quantum-runtime")
	if err != nil || !ok {
		t.Fatalf("lookup failed: %v %v", ok, err)
	}
	if component.Ownership != OwnershipUnknown {
		t.Fatalf("ownership=%s", component.Ownership)
	}
}

func TestSecretLikeVersionTextIsNeverReturned(t *testing.T) {
	observer := &fixtureObserver{fixture: fixture{
		Paths: []string{"/opt/ember-coreui"},
		Files: map[string]string{"/opt/ember-coreui/VERSION": "password=supersecret"},
	}}
	component, _, err := NewScanner(observer).Component(context.Background(), "ember-coreui")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(component)
	if strings.Contains(strings.ToLower(string(data)), "supersecret") {
		t.Fatalf("secret leaked: %s", data)
	}
}

func TestProbeSurfaceCannotInvokeShell(t *testing.T) {
	observer := loadFixture(t, "starlight")
	if _, err := NewScanner(observer).Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range observer.calls {
		if strings.HasPrefix(call, "command:sh ") || strings.HasPrefix(call, "command:bash ") {
			t.Fatalf("shell probe invoked: %s", call)
		}
	}
}

func findComponent(t *testing.T, snapshot Snapshot, id string) Component {
	t.Helper()
	for _, component := range snapshot.Components {
		if component.ID == id {
			return component
		}
	}
	t.Fatalf("component %s not found", id)
	return Component{}
}
