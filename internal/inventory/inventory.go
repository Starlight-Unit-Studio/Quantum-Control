package inventory

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "quantum.control/component-inventory/v1alpha1"

type Ownership string

const (
	OwnershipManaged  Ownership = "managed"
	OwnershipExternal Ownership = "external"
	OwnershipDisabled Ownership = "disabled"
	OwnershipUnknown  Ownership = "unknown"
)

type Health string

const (
	HealthHealthy  Health = "healthy"
	HealthDegraded Health = "degraded"
	HealthDisabled Health = "disabled"
	HealthUnknown  Health = "unknown"
)

type Listener struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type Evidence struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Detail string `json:"detail,omitempty"`
}

type Component struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Category     string     `json:"category"`
	Version      string     `json:"version"`
	Ownership    Ownership  `json:"ownership"`
	Health       Health     `json:"health"`
	Services     []string   `json:"services"`
	Listeners    []Listener `json:"listeners"`
	Roots        []string   `json:"roots"`
	Capabilities []string   `json:"capabilities"`
	Evidence     []Evidence `json:"evidence"`
	ObservedAt   time.Time  `json:"observed_at"`
	Warnings     []string   `json:"warnings"`
}

type Snapshot struct {
	Schema     string      `json:"schema"`
	ObservedAt time.Time   `json:"observed_at"`
	Components []Component `json:"components"`
}

type Scanner interface {
	Snapshot(context.Context) (Snapshot, error)
	Component(context.Context, string) (Component, bool, error)
}

type ServiceState struct {
	Unit        string
	LoadState   string
	ActiveState string
	SubState    string
}

// Observer exposes only bounded, read-only primitives. Every command, path and
// glob originates in fixed component definitions rather than public input.
type Observer interface {
	PathExists(string) (bool, error)
	ReadFile(string, int64) ([]byte, error)
	Command(context.Context, string, ...string) (string, error)
	Service(context.Context, string) (ServiceState, error)
	Glob(string) ([]string, error)
}

type NativeObserver struct{}

func (NativeObserver) PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (NativeObserver) ReadFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file exceeds read limit")
	}
	return data, nil
}

func (NativeObserver) Command(ctx context.Context, name string, args ...string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", errors.New("command unavailable")
	}
	command := exec.CommandContext(ctx, path, args...)
	data, err := command.CombinedOutput()
	if err != nil {
		return "", errors.New("fixed probe command failed")
	}
	if len(data) > 16<<10 {
		data = data[:16<<10]
	}
	return string(data), nil
}

func (NativeObserver) Service(ctx context.Context, unit string) (ServiceState, error) {
	path, err := exec.LookPath("systemctl")
	if err != nil {
		return ServiceState{}, errors.New("systemctl unavailable")
	}
	data, err := exec.CommandContext(ctx, path, "show", "--no-page", "--property=Id,LoadState,ActiveState,SubState", unit).Output()
	if err != nil {
		return ServiceState{}, errors.New("systemctl fixed probe failed")
	}
	state := ServiceState{Unit: unit}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "Id":
			state.Unit = value
		case "LoadState":
			state.LoadState = value
		case "ActiveState":
			state.ActiveState = value
		case "SubState":
			state.SubState = value
		}
	}
	if err := scanner.Err(); err != nil {
		return ServiceState{}, errors.New("invalid systemctl response")
	}
	return state, nil
}

func (NativeObserver) Glob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

type versionProbe struct {
	command string
	args    []string
}

type definition struct {
	ID            string
	Name          string
	Category      string
	Paths         []string
	ManagedMarker string
	VersionFiles  []string
	Versions      []versionProbe
	Services      []string
	ServiceGlobs  []string
	Globs         []string
	Capabilities  []string
}

type Service struct {
	observer    Observer
	definitions []definition
	now         func() time.Time
}

func NewScanner(observer Observer) *Service {
	if observer == nil {
		observer = NativeObserver{}
	}
	return &Service{observer: observer, definitions: defaultDefinitions(), now: time.Now}
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	observed := s.now().UTC()
	components := make([]Component, 0, len(s.definitions))
	for _, definition := range s.definitions {
		component, err := s.scanDefinition(ctx, definition, observed)
		if err != nil {
			return Snapshot{}, err
		}
		components = append(components, component)
	}
	sort.Slice(components, func(i, j int) bool { return components[i].ID < components[j].ID })
	return Snapshot{Schema: SchemaVersion, ObservedAt: observed, Components: components}, nil
}

func (s *Service) Component(ctx context.Context, id string) (Component, bool, error) {
	for _, definition := range s.definitions {
		if definition.ID != id {
			continue
		}
		component, err := s.scanDefinition(ctx, definition, s.now().UTC())
		return component, true, err
	}
	return Component{}, false, nil
}

func (s *Service) scanDefinition(ctx context.Context, definition definition, observed time.Time) (Component, error) {
	if err := ctx.Err(); err != nil {
		return Component{}, err
	}
	component := Component{
		ID: definition.ID, Name: definition.Name, Category: definition.Category,
		Ownership: OwnershipDisabled, Health: HealthDisabled,
		Services: []string{}, Listeners: []Listener{}, Roots: []string{},
		Capabilities: append([]string{}, definition.Capabilities...), Evidence: []Evidence{},
		ObservedAt: observed, Warnings: []string{},
	}
	positiveEvidence := 0
	nonMarkerEvidence := 0
	probeErrors := 0
	managedMarker := false

	for _, path := range definition.Paths {
		exists, err := s.observer.PathExists(path)
		if err != nil {
			probeErrors++
			continue
		}
		if !exists {
			continue
		}
		positiveEvidence++
		nonMarkerEvidence++
		component.Roots = appendUnique(component.Roots, path)
		component.Evidence = append(component.Evidence, Evidence{Kind: "path", Source: path, Detail: "present"})
	}
	if definition.ManagedMarker != "" {
		exists, err := s.observer.PathExists(definition.ManagedMarker)
		if err != nil {
			probeErrors++
		} else if exists {
			positiveEvidence++
			managedMarker = true
			component.Evidence = append(component.Evidence, Evidence{Kind: "ownership-marker", Source: definition.ManagedMarker, Detail: "present"})
		}
	}
	for _, pattern := range definition.Globs {
		matches, err := s.observer.Glob(pattern)
		if err != nil {
			probeErrors++
			continue
		}
		for _, match := range matches {
			positiveEvidence++
			nonMarkerEvidence++
			component.Evidence = append(component.Evidence, Evidence{Kind: "filesystem", Source: match, Detail: "present"})
		}
	}

	activeService := false
	inactiveService := false
	serviceNames := append([]string{}, definition.Services...)
	for _, pattern := range definition.ServiceGlobs {
		matches, err := s.observer.Glob(pattern)
		if err != nil {
			probeErrors++
			continue
		}
		for _, match := range matches {
			serviceNames = appendUnique(serviceNames, filepath.Base(match))
		}
	}
	for _, unit := range serviceNames {
		state, err := s.observer.Service(ctx, unit)
		if err != nil {
			probeErrors++
			continue
		}
		if state.LoadState == "" || state.LoadState == "not-found" {
			continue
		}
		positiveEvidence++
		nonMarkerEvidence++
		component.Services = appendUnique(component.Services, state.Unit)
		component.Evidence = append(component.Evidence, Evidence{Kind: "service", Source: state.Unit, Detail: state.LoadState + "/" + state.ActiveState + "/" + state.SubState})
		if state.ActiveState == "active" {
			activeService = true
		} else {
			inactiveService = true
		}
	}

	versions := make([]string, 0, 2)
	for _, file := range definition.VersionFiles {
		exists, err := s.observer.PathExists(file)
		if err != nil {
			probeErrors++
			continue
		}
		if !exists {
			continue
		}
		data, err := s.observer.ReadFile(file, 4096)
		if err != nil {
			probeErrors++
			continue
		}
		value := cleanVersion(string(data))
		if value == "" {
			continue
		}
		versions = appendUnique(versions, value)
		positiveEvidence++
		nonMarkerEvidence++
		component.Evidence = append(component.Evidence, Evidence{Kind: "version-file", Source: file, Detail: value})
	}
	for _, probe := range definition.Versions {
		output, err := s.observer.Command(ctx, probe.command, probe.args...)
		if err != nil {
			continue
		}
		value := cleanVersion(output)
		if value == "" {
			continue
		}
		versions = appendUnique(versions, value)
		positiveEvidence++
		nonMarkerEvidence++
		component.Evidence = append(component.Evidence, Evidence{Kind: "version", Source: probe.command, Detail: value})
	}
	if len(versions) == 1 {
		component.Version = versions[0]
	}
	if len(versions) > 1 {
		component.Warnings = append(component.Warnings, "conflicting version evidence was withheld")
	}

	switch {
	case managedMarker && nonMarkerEvidence == 0:
		component.Ownership = OwnershipUnknown
		component.Health = HealthUnknown
		component.Warnings = append(component.Warnings, "managed marker exists without confirming runtime evidence")
	case managedMarker:
		component.Ownership = OwnershipManaged
	case positiveEvidence > 0:
		component.Ownership = OwnershipExternal
	case probeErrors > 0:
		component.Ownership = OwnershipUnknown
		component.Health = HealthUnknown
		component.Warnings = append(component.Warnings, "one or more read-only probes were inconclusive")
	default:
		component.Ownership = OwnershipDisabled
		component.Health = HealthDisabled
	}
	if component.Ownership == OwnershipManaged || component.Ownership == OwnershipExternal {
		switch {
		case activeService:
			component.Health = HealthHealthy
		case inactiveService:
			component.Health = HealthDegraded
		default:
			component.Health = HealthUnknown
		}
		if probeErrors > 0 {
			component.Warnings = append(component.Warnings, "some optional read-only probes were unavailable")
		}
	}
	sort.Strings(component.Services)
	sort.Strings(component.Roots)
	sort.Strings(component.Capabilities)
	return component, nil
}

var versionPattern = regexp.MustCompile(`(?i)\b(?:v)?([0-9]+(?:\.[0-9]+){1,3}(?:[-+._][0-9A-Za-z.-]+)?)\b`)

func cleanVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if match := versionPattern.FindStringSubmatch(value); len(match) > 1 {
		return strings.TrimPrefix(match[1], "v")
	}
	if len(value) > 128 {
		value = value[:128]
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"password", "token", "secret", "credential"} {
		if strings.Contains(lower, forbidden) {
			return ""
		}
	}
	return value
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func defaultDefinitions() []definition {
	return []definition{
		{ID: "keyhelp", Name: "KeyHelp", Category: "hosting-control", Paths: []string{"/etc/keyhelp", "/home/keyhelp"}, Services: []string{"keyhelp.service"}, Capabilities: []string{"databases", "domains", "hosting-control", "tls"}},
		{ID: "apache", Name: "Apache HTTP Server", Category: "web-server", Paths: []string{"/etc/apache2", "/etc/httpd"}, Versions: []versionProbe{{command: "apache2", args: []string{"-v"}}, {command: "httpd", args: []string{"-v"}}}, Services: []string{"apache2.service", "httpd.service"}, Capabilities: []string{"http", "reverse-proxy"}},
		{ID: "nginx", Name: "Nginx", Category: "web-server", Paths: []string{"/etc/nginx"}, Versions: []versionProbe{{command: "nginx", args: []string{"-v"}}}, Services: []string{"nginx.service"}, Capabilities: []string{"http", "reverse-proxy"}},
		{ID: "php", Name: "PHP / PHP-FPM", Category: "application-runtime", Paths: []string{"/etc/php"}, Versions: []versionProbe{{command: "php", args: []string{"-v"}}}, Services: []string{"php-fpm.service"}, ServiceGlobs: []string{"/etc/systemd/system/php*-fpm.service", "/lib/systemd/system/php*-fpm.service", "/usr/lib/systemd/system/php*-fpm.service"}, Globs: []string{"/run/php/php*-fpm.sock"}, Capabilities: []string{"fastcgi", "php"}},
		{ID: "mariadb", Name: "MariaDB / MySQL", Category: "database", Paths: []string{"/etc/mysql", "/var/lib/mysql"}, Versions: []versionProbe{{command: "mariadb", args: []string{"--version"}}, {command: "mysql", args: []string{"--version"}}}, Services: []string{"mariadb.service", "mysql.service"}, Capabilities: []string{"sql"}},
		{ID: "postgresql", Name: "PostgreSQL", Category: "database", Paths: []string{"/etc/postgresql", "/var/lib/postgresql"}, Versions: []versionProbe{{command: "psql", args: []string{"--version"}}}, Services: []string{"postgresql.service"}, Capabilities: []string{"sql"}},
		{ID: "container-runtime", Name: "Container Runtime", Category: "containers", Paths: []string{"/var/lib/docker", "/var/lib/containers"}, Versions: []versionProbe{{command: "docker", args: []string{"--version"}}, {command: "podman", args: []string{"--version"}}}, Services: []string{"docker.service", "podman.service"}, Capabilities: []string{"containers"}},
		{ID: "ollama", Name: "Ollama", Category: "ai-runtime", Paths: []string{"/usr/local/bin/ollama", "/usr/bin/ollama"}, Versions: []versionProbe{{command: "ollama", args: []string{"--version"}}}, Services: []string{"ollama.service"}, Capabilities: []string{"local-inference"}},
		{ID: "quantum-runtime", Name: "Quantum Runtime", Category: "ai-runtime", Paths: []string{"/usr/local/bin/quantum-runtime", "/etc/quantum-runtime"}, ManagedMarker: "/etc/quantum-runtime/.managed.json", Services: []string{"quantum-runtime.service"}, Capabilities: []string{"local-inference", "model-registry", "streaming"}},
		{ID: "searxng", Name: "SearXNG", Category: "search", Paths: []string{"/etc/searxng", "/opt/searxng"}, Versions: []versionProbe{{command: "searxng", args: []string{"--version"}}}, Services: []string{"searxng.service"}, Capabilities: []string{"web-search"}},
		{ID: "ember-coreui", Name: "Ember CoreUI", Category: "application", Paths: []string{"/opt/ember-coreui", "/srv/ember-coreui", "/var/lib/ember-coreui"}, VersionFiles: []string{"/opt/ember-coreui/VERSION", "/srv/ember-coreui/VERSION"}, Services: []string{"ember-coreui.service"}, Capabilities: []string{"ai-ui"}},
		{ID: "starlight-unit-game", Name: "STΛRLIGHT UNIT Game/Repack", Category: "application", Paths: []string{"/opt/starlight-unit-game", "/srv/starlight-unit-game", "/var/www/starlight-unit"}, VersionFiles: []string{"/opt/starlight-unit-game/VERSION", "/srv/starlight-unit-game/VERSION"}, Services: []string{"starlight-unit-game.service", "ember-worker.service", "ember-worker-media.service"}, Capabilities: []string{"ember-workers", "game"}},
	}
}
