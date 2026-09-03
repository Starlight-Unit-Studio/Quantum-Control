package systemprobe

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Probe provides fixed, read-only system observations to qcored.
type Probe interface {
	Snapshot(context.Context) (map[string]any, error)
	ServiceStatus(context.Context, string) (map[string]any, error)
}

// Native reads local Unix/Linux system state without accepting shell text.
type Native struct{}

// Snapshot returns a bounded machine summary.
func (Native) Snapshot(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("read hostname: %w", err)
	}
	result := map[string]any{
		"hostname":     hostname,
		"goos":         runtime.GOOS,
		"architecture": runtime.GOARCH,
		"cpu_count":    runtime.NumCPU(),
		"observed_at":  time.Now().UTC(),
	}
	for key, value := range readOSRelease("/etc/os-release") {
		result[key] = value
	}
	if uptime, err := readUptime("/proc/uptime"); err == nil {
		result["uptime_seconds"] = uptime
	}
	return result, nil
}

// ServiceStatus invokes systemctl with a fixed argument vector. The unit name
// is validated by the operation registry before this method is called.
func (Native) ServiceStatus(ctx context.Context, unit string) (map[string]any, error) {
	path, err := exec.LookPath("systemctl")
	if err != nil {
		return nil, errors.New("systemctl is not available")
	}
	command := exec.CommandContext(ctx, path,
		"show",
		"--no-page",
		"--property=Id,LoadState,ActiveState,SubState,UnitFileState",
		unit,
	)
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("systemctl status failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("run systemctl: %w", err)
	}
	result := map[string]any{"unit": unit}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok && key != "" {
			result[toSnakeCase(key)] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse systemctl output: %w", err)
	}
	return result, nil
}

func readOSRelease(path string) map[string]string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = parseOSReleaseValue(value)
		switch key {
		case "ID":
			result["os_id"] = value
		case "VERSION_ID":
			result["os_version"] = value
		case "PRETTY_NAME":
			result["os_name"] = value
		}
	}
	return result
}

func parseOSReleaseValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return strings.Trim(value, "\"")
}

func readUptime(path string) (float64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(data)), " ")
	return strconv.ParseFloat(first, 64)
}

func toSnakeCase(value string) string {
	var result strings.Builder
	for i, r := range value {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteByte('_')
			}
			result.WriteRune(r + ('a' - 'A'))
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
}
