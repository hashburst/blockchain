package main

import (
	"context"
	"errors"
	"hashburst/blockchain"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type controlledRunner struct {
	starts    atomic.Int32
	running   atomic.Bool
	entered   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
	fail      bool
}

func (c *controlledRunner) Start() error {
	if c.fail {
		return errors.New("journal unavailable")
	}
	c.starts.Add(1)
	c.running.Store(true)
	return nil
}
func (c *controlledRunner) Run(ctx context.Context) error {
	select {
	case c.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	c.running.Store(false)
	select {
	case c.cancelled <- struct{}{}:
	default:
	}
	<-c.release
	return ctx.Err()
}
func (c *controlledRunner) Running() bool { return c.running.Load() }
func (c *controlledRunner) TryStatus() (blockchain.ConsensusReactorStatus, bool) {
	return blockchain.ConsensusReactorStatus{Running: c.Running()}, true
}
func await(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("lifecycle deadline")
	}
}
func TestLifecycleStopWaitsBeforeRestart(t *testing.T) {
	c := &controlledRunner{entered: make(chan struct{}, 2), cancelled: make(chan struct{}, 2), release: make(chan struct{})}
	rt := &runtime{reactor: c}
	start := func() { rt.startConsensus(httptest.NewRecorder(), httptest.NewRequest("POST", "/control/start", nil)) }
	start()
	await(t, c.entered)
	stopped := make(chan struct{})
	go func() { rt.stopConsensusInternal(); close(stopped) }()
	await(t, c.cancelled)
	restarted := make(chan struct{})
	go func() { start(); close(restarted) }()
	select {
	case <-stopped:
		t.Fatal("stop acknowledged before Run exited")
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-restarted:
		t.Fatal("restart acknowledged during old Run cleanup")
	case <-time.After(30 * time.Millisecond):
	}
	if c.starts.Load() != 1 {
		t.Fatal("overlapping start")
	}
	close(c.release)
	await(t, stopped)
	await(t, restarted)
	await(t, c.entered)
	if c.starts.Load() != 2 || !c.Running() {
		t.Fatal("new generation not running")
	}
	rt.stopConsensusInternal()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.started || rt.consCancel != nil {
		t.Fatal("stale runtime state")
	}
}
func TestLifecycleStartFailureAndIdempotence(t *testing.T) {
	c := &controlledRunner{fail: true, entered: make(chan struct{}, 2), cancelled: make(chan struct{}, 2), release: make(chan struct{})}
	rt := &runtime{reactor: c}
	w := httptest.NewRecorder()
	rt.startConsensus(w, httptest.NewRequest("POST", "/control/start", nil))
	if w.Code != 409 || rt.started {
		t.Fatal("failed start acknowledged")
	}
	c.fail = false
	for i := 0; i < 2; i++ {
		rt.startConsensus(httptest.NewRecorder(), httptest.NewRequest("POST", "/control/start", nil))
	}
	await(t, c.entered)
	if c.starts.Load() != 1 {
		t.Fatal("duplicate start")
	}
	close(c.release)
	rt.stopConsensusInternal()
	rt.stopConsensusInternal()
}
