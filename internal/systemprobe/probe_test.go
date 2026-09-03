package systemprobe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Native{}).Snapshot(ctx); err == nil {
		t.Fatal("Snapshot ignored a canceled context")
	}
}

func TestReadOSReleaseParsesQuotedValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-release")
	content := "ID=quantum\nVERSION_ID=\"0.1\"\nPRETTY_NAME=\"Quantum CoreOS \\\"Foundation\\\"\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result := readOSRelease(path)
	if result["os_id"] != "quantum" || result["os_version"] != "0.1" {
		t.Fatalf("unexpected basic fields: %#v", result)
	}
	if result["os_name"] != `Quantum CoreOS "Foundation"` {
		t.Fatalf("unexpected pretty name: %q", result["os_name"])
	}
}

func TestToSnakeCase(t *testing.T) {
	if got := toSnakeCase("ActiveState"); got != "active_state" {
		t.Fatalf("unexpected conversion: %q", got)
	}
}
