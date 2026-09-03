package broker

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListenUnixRefusesRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qcored.sock")
	if err := os.WriteFile(path, []byte("do not replace"), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := ListenUnix(path)
	if listener != nil {
		_ = listener.Close()
		t.Fatal("ListenUnix returned a listener for a regular file path")
	}
	if !errors.Is(err, errInvalidSocket) {
		t.Fatalf("unexpected error: %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil || string(data) != "do not replace" {
		t.Fatalf("regular file was changed: data=%q err=%v", data, readErr)
	}
}

func TestListenUnixCreatesProtectedSocketAndCleansUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qcored.sock")
	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("path is not a Unix socket: %v", info.Mode())
	}
	if info.Mode().Perm() != 0o660 {
		t.Fatalf("unexpected socket permissions: %o", info.Mode().Perm())
	}
	connection, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket path remains after close: %v", err)
	}
}

func TestListenUnixReplacesVerifiedStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qcored.sock")
	address, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := net.ListenUnix("unix", address)
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("stale socket path missing: %v", err)
	}

	listener, err := ListenUnix(path)
	if err != nil {
		t.Fatalf("ListenUnix did not replace stale socket: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}
