package execution

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestStoreLockProcessExclusion(t *testing.T) {
	if path := os.Getenv("HVM_EVM_LOCK_CHILD"); path != "" {
		f, err := os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			os.Exit(4)
		}
		defer f.Close()
		if lockExclusive(f) == nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err = lockExclusive(f); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStoreLockProcessExclusion$")
	cmd.Env = append(os.Environ(), "HVM_EVM_LOCK_CHILD="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("second process admitted: %v %s", err, out)
	}
	f.Close()
	next, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if err = lockExclusive(next); err != nil {
		t.Fatalf("reopen after close: %v", err)
	}
}
