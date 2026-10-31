package ui

// asyncRunner carries one page's backend answers from a worker goroutine to
// the UI thread: run computes the call off-thread, tick applies at most one
// queued answer per frame. The pages keep their own busy gates and c.After
// polling; this only replaces the channel-plus-goroutine boilerplate. The
// channel is never closed - it lives as long as the state that owns it, and
// that state lives as long as the process.
type asyncRunner[T any] struct {
	ch chan T
}

// startAsync builds the runner with the page's usual buffer.
func startAsync[T any](buffer int) asyncRunner[T] {
	return asyncRunner[T]{ch: make(chan T, buffer)}
}

// run computes f on its own goroutine and queues what it returns: a closure
// that applies the outcome, or a plain result struct the tick unwraps.
func (r *asyncRunner[T]) run(f func() T) {
	go func() { r.ch <- f() }()
}

// tick applies one queued answer, if any, and reports whether it did. At most
// one per call, matching the per-frame selects it replaces. Ch exposes the
// lane for a test's timeout, which needs the blocking receive.
func (r *asyncRunner[T]) tick(apply func(T)) bool {
	select {
	case v := <-r.ch:
		apply(v)
		return true
	default:
		return false
	}
}

// Ch exposes the underlying channel; tests use it to arm a timeout on a
// blocking receive.
func (r *asyncRunner[T]) Ch() chan T { return r.ch }
