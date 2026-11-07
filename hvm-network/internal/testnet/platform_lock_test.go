package testnet

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRuntimeLockProcessExclusion(t *testing.T) {
	if path := os.Getenv("HVM_LOCK_CHILD"); path != "" {
		f, err := openRuntimeLock(path)
		if err == nil {
			f.Close()
			os.Exit(3)
		}
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "runtime.lock")
	f, err := openRuntimeLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRuntimeLockProcessExclusion$")
	cmd.Env = append(os.Environ(), "HVM_LOCK_CHILD="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("second process admitted: %v %s", err, out)
	}
	if err := unlockRuntime(f); err != nil {
		t.Fatal(err)
	}
	f.Close()
	next, err := openRuntimeLock(path)
	if err != nil {
		t.Fatalf("reopen after release: %v", err)
	}
	next.Close()
}
