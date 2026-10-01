// Package diagnostics provides opt-in, time-bounded native Go profiles without
// exposing HTTP endpoints. Do not run multiple intrusive profilers together.
package diagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"
)

var sessionMu sync.Mutex
var sessionActive bool

// Start creates a new private output directory (must not exist) and automatically
// finishes after duration. kind is cpu, heap, block, mutex, or trace. Heap retains
// Go's default sampling rate. Trace should be short (e.g. 5 seconds).
func Start(dir, kind string, duration time.Duration) (func() error, error) {
	if dir == "" {
		return func() error { return nil }, nil
	}
	if duration <= 0 || duration > 5*time.Minute {
		return nil, errors.New("profile duration must be 1ns..5m")
	}
	switch kind {
	case "cpu", "heap", "block", "mutex", "trace":
	default:
		return nil, errors.New("unknown profile kind")
	}
	sessionMu.Lock()
	if sessionActive {
		sessionMu.Unlock()
		return nil, errors.New("profile session already active")
	}
	sessionActive = true
	sessionMu.Unlock()
	release := func() { sessionMu.Lock(); sessionActive = false; sessionMu.Unlock() }
	if e := os.Mkdir(dir, 0700); e != nil {
		release()
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, kind+".pprof"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		release()
		return nil, e
	}
	oldMutex := 0
	switch kind {
	case "cpu":
		e = pprof.StartCPUProfile(f)
	case "trace":
		e = trace.Start(f)
	case "block":
		runtime.SetBlockProfileRate(1000000)
	case "mutex":
		oldMutex = runtime.SetMutexProfileFraction(100)
	}
	if e != nil {
		f.Close()
		release()
		return nil, e
	}
	var once sync.Once
	var finalErr error
	finish := func() error {
		once.Do(func() {
			defer release()
			switch kind {
			case "cpu":
				pprof.StopCPUProfile()
			case "trace":
				trace.Stop()
			case "block":
				runtime.SetBlockProfileRate(0)
				finalErr = pprof.Lookup("block").WriteTo(f, 0)
			case "mutex":
				runtime.SetMutexProfileFraction(oldMutex)
				finalErr = pprof.Lookup("mutex").WriteTo(f, 0)
			case "heap":
				finalErr = pprof.Lookup("heap").WriteTo(f, 0)
			}
			finalErr = errors.Join(finalErr, f.Close())
		})
		return finalErr
	}
	timer := time.AfterFunc(duration, func() { finish() })
	return func() error { timer.Stop(); return finish() }, nil
}
