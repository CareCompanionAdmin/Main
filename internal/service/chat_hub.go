package service

import (
	"sync"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// ChatHub fans out new chat messages to live SSE subscribers.
//
// Why an in-memory hub instead of polling: the chat UI has to feel like a
// conversation, so a recipient seeing a new message after a 30s interval is
// not acceptable. SSE is a better fit than WebSocket here because it's
// one-way (server → client), works through ALB without an Upgrade dance,
// and reconnects automatically in the browser.
//
// One-process scope: this works because dev runs a single Go binary and
// prod runs an autoscaling group where every connected client lands on one
// instance. If we ever need cross-instance fanout, replace the in-memory
// map with a Redis pub/sub bridge — the public API of Subscribe/Broadcast
// stays the same.
type ChatHub struct {
	mu   sync.RWMutex
	subs map[uuid.UUID]map[*ChatSubscriber]struct{} // threadID -> set
}

// ChatSubscriber holds the channel a single SSE connection drains.
// Capacity 16 is enough that a slow client momentarily stalling doesn't
// block other recipients on the same thread; if the buffer fills, the
// hub drops the message rather than blocking the broadcast.
type ChatSubscriber struct {
	UserID uuid.UUID
	Ch     chan *models.ChatMessage
}

func NewChatHub() *ChatHub {
	return &ChatHub{
		subs: make(map[uuid.UUID]map[*ChatSubscriber]struct{}),
	}
}

// Subscribe registers a new subscriber for the given thread and returns
// the subscriber and an unsubscribe func. Callers must call the
// unsubscribe func when their connection ends.
func (h *ChatHub) Subscribe(threadID, userID uuid.UUID) (*ChatSubscriber, func()) {
	sub := &ChatSubscriber{
		UserID: userID,
		Ch:     make(chan *models.ChatMessage, 16),
	}

	h.mu.Lock()
	set, ok := h.subs[threadID]
	if !ok {
		set = make(map[*ChatSubscriber]struct{})
		h.subs[threadID] = set
	}
	set[sub] = struct{}{}
	h.mu.Unlock()

	unsub := func() {
		h.mu.Lock()
		if s, ok := h.subs[threadID]; ok {
			delete(s, sub)
			if len(s) == 0 {
				delete(h.subs, threadID)
			}
		}
		h.mu.Unlock()
		close(sub.Ch)
	}
	return sub, unsub
}

// Broadcast pushes a message to every subscriber of the thread. Drops on
// full buffer rather than blocking — a stuck client must not freeze the
// fanout for everyone else.
func (h *ChatHub) Broadcast(threadID uuid.UUID, msg *models.ChatMessage) {
	h.mu.RLock()
	set := h.subs[threadID]
	subs := make([]*ChatSubscriber, 0, len(set))
	for s := range set {
		subs = append(subs, s)
	}
	h.mu.RUnlock()

	for _, s := range subs {
		select {
		case s.Ch <- msg:
		default:
			// buffer full — drop. The client will see the message
			// on next page reload anyway.
		}
	}
}
