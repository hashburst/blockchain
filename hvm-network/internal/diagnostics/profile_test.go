package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileLifecycle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profile")
	stop, e := Start(dir, "heap", time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Start(dir+"2", "cpu", time.Second); e == nil {
		t.Fatal("concurrent session")
	}
	if e = stop(); e != nil {
		t.Fatal(e)
	}
	if e = stop(); e != nil {
		t.Fatal(e)
	}
	s, e := os.Stat(filepath.Join(dir, "heap.pprof"))
	if e != nil || s.Size() == 0 {
		t.Fatal(e)
	}
	if _, e = Start(dir, "heap", time.Second); e == nil {
		t.Fatal("overwrote existing output")
	}
}
