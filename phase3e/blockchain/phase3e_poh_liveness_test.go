package blockchain

import "testing"

func TestPhase3EDefaultPoHTicksUnchanged(t *testing.T) {
	cfg := DefaultProtocolV2Config()
	if got := cfg.EffectivePoHTicks(); got != PoHTicks {
		t.Fatalf("default PoH ticks=%d want %d", got, PoHTicks)
	}
	var zero ProtocolV2Config
	if got := zero.EffectivePoHTicks(); got != PoHTicks {
		t.Fatalf("zero-value PoH ticks=%d want compatibility default %d", got, PoHTicks)
	}
}

func TestPhase3EProductionPoHVectorsUnchanged(t *testing.T) {
	vectors := []struct {
		prev int64
		want int64
	}{
		{0, -5525551911051244016},
		{-5525551911051244016, -4533409688973210426},
	}
	for _, v := range vectors {
		if got := PoH(v.prev); got != v.want {
			t.Fatalf("PoH(%d)=%d want %d", v.prev, got, v.want)
		}
		if !ValidatePoH(v.prev, v.want) {
			t.Fatalf("historical PoH vector rejected for prev=%d", v.prev)
		}
	}
}

func TestPhase3EIsolatedPoHTicksAreConfigBound(t *testing.T) {
	cfg := DefaultProtocolV2Config()
	cfg.PoHTicksPerBlock = 4_000
	if err := cfg.Validate(); err != nil {
		t.Fatalf("isolated PoH config rejected: %v", err)
	}
	got := poHWithTicks(0, cfg.EffectivePoHTicks())
	if !validatePoHWithTicks(0, got, cfg.EffectivePoHTicks()) {
		t.Fatal("configured PoH value did not validate")
	}
	if got == PoH(0) {
		t.Fatal("isolated reduced-tick PoH unexpectedly equals production vector")
	}
	if validatePoHWithTicks(0, got, PoHTicks) {
		t.Fatal("reduced-tick PoH unexpectedly validated under production ticks")
	}
}

func TestPhase3EPoHTickBoundsFailClosed(t *testing.T) {
	for _, ticks := range []int{-1, PoHTicks + 1} {
		cfg := DefaultProtocolV2Config()
		cfg.PoHTicksPerBlock = ticks
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid PoH ticks=%d unexpectedly accepted", ticks)
		}
	}
}
