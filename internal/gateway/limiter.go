package gateway

import (
	"context"
	"lam.local/lam2/internal/fault"
	"sync"
)

type Capacity struct{ Active, Queued int }
type waiter struct {
	id      string
	ready   chan struct{}
	granted bool
}
type Limiter struct {
	mu                           sync.Mutex
	max, per, maxQ, perQ, active int
	counts                       map[string]int
	queue                        []*waiter
}

func NewLimiter(max, per, maxQ, perQ int) *Limiter {
	return &Limiter{max: max, per: per, maxQ: maxQ, perQ: perQ, counts: map[string]int{}}
}
func (l *Limiter) Acquire(ctx context.Context, id string) (func(), error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	l.mu.Lock()
	w := &waiter{id: id, ready: make(chan struct{})}
	if l.active < l.max && l.counts[id] < l.per && len(l.queue) == 0 {
		l.grant(w)
	} else {
		n := 0
		for _, q := range l.queue {
			if q.id == id {
				n++
			}
		}
		if len(l.queue) >= l.maxQ || n >= l.perQ {
			l.mu.Unlock()
			return nil, fault.New("GATEWAY_BUSY", "Gateway queue capacity exceeded")
		}
		l.queue = append(l.queue, w)
		l.dispatch()
	}
	l.mu.Unlock()
	select {
	case <-w.ready:
		if ctx.Err() != nil {
			l.release(id)
			return nil, ctx.Err()
		}
		var once sync.Once
		return func() { once.Do(func() { l.release(id) }) }, nil
	case <-ctx.Done():
		l.mu.Lock()
		if w.granted {
			l.active--
			l.counts[id]--
			if l.counts[id] == 0 {
				delete(l.counts, id)
			}
		} else {
			for i, q := range l.queue {
				if q == w {
					l.queue = append(l.queue[:i], l.queue[i+1:]...)
					break
				}
			}
		}
		l.dispatch()
		l.mu.Unlock()
		return nil, ctx.Err()
	}
}
func (l *Limiter) grant(w *waiter) { w.granted = true; l.active++; l.counts[w.id]++; close(w.ready) }
func (l *Limiter) dispatch() {
	for i := 0; i < len(l.queue) && l.active < l.max; {
		w := l.queue[i]
		if l.counts[w.id] >= l.per {
			i++
			continue
		}
		l.queue = append(l.queue[:i], l.queue[i+1:]...)
		l.grant(w)
	}
}
func (l *Limiter) release(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active--
	l.counts[id]--
	if l.counts[id] == 0 {
		delete(l.counts, id)
	}
	l.dispatch()
}
func (l *Limiter) Snapshot() Capacity {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Capacity{l.active, len(l.queue)}
}
