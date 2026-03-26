package jsexec

import "github.com/dop251/goja"

// Runtime wraps a goja.Runtime and adds channel-based interrupt
// capabilities. Blocking Go operations (e.g. Utils.sleep) can
// select on DoneCh() to be cancelled promptly when Interrupt is called.
type Runtime struct {
	*goja.Runtime
	doneCh chan struct{}
}

// NewRuntime creates a new Runtime with a fresh goja.Runtime and
// an open doneCh channel.
func NewRuntime() *Runtime {
	return &Runtime{
		Runtime: goja.New(),
		doneCh:  make(chan struct{}),
	}
}

// Interrupt interrupts the goja VM and closes the doneCh channel
// so that any blocking Go operations can be cancelled. Safe to
// call from any goroutine. Subsequent calls are safe (doneCh
// close is guarded).
func (r *Runtime) Interrupt(v interface{}) {
	r.Runtime.Interrupt(v)
	select {
	case <-r.doneCh:
		// already closed
	default:
		close(r.doneCh)
	}
}

// DoneCh returns a channel that is closed when Interrupt is called.
func (r *Runtime) DoneCh() <-chan struct{} {
	return r.doneCh
}
