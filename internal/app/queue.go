package app

import (
	"sync"

	"github.com/goshitsarch-eng/Gosh-Slack-CLI-Manager/internal/msg"
)

// queue is an unbounded, goroutine-safe message queue. Background tasks push
// into it and the UI loop drains it between frames.
type queue struct {
	mu    sync.Mutex
	items []msg.Message
}

func (q *queue) send(m msg.Message) {
	q.mu.Lock()
	q.items = append(q.items, m)
	q.mu.Unlock()
}

func (q *queue) tryRecv() (msg.Message, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return nil, false
	}
	m := q.items[0]
	q.items[0] = nil
	q.items = q.items[1:]
	return m, true
}
