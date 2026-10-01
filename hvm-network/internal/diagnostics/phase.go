package diagnostics

import (
	"context"
	"runtime/pprof"
	"runtime/trace"
)

// Phase labels CPU samples and execution traces. It does not alter scheduling.
func Phase(name string, work func() error) (err error) {
	pprof.Do(context.Background(), pprof.Labels("hvm_stage", name), func(ctx context.Context) {
		trace.WithRegion(ctx, name, func() { err = work() })
	})
	return err
}
