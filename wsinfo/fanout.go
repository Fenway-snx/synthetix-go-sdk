package wsinfo

import (
	"sync"
	"sync/atomic"

	sdklogger "github.com/Fenway-snx/synthetix-go-sdk/logger"
	"github.com/Fenway-snx/synthetix-go-sdk/types"
)

// Handler is the per-message callback invoked for each notification
// delivered to a local subscriber. It runs on a dedicated
// per-subscriber goroutine: a slow Handler does NOT block the read
// loop or other subscribers. It CAN cause its own events to be
// dropped (drop-oldest backpressure).
//
// Handler must be non-nil. It should return quickly; any significant
// work should be dispatched to another goroutine.
type Handler func(msg *types.WSMessage)

// defaultSubscriberBufferSize is the ring-buffer depth applied when
// a Client is constructed without an explicit SubscriberBufferSize.
// 256 messages is generous for every public /v1/ws/info stream we
// care about today.
const defaultSubscriberBufferSize = 256

// subscriber is one local fan-out handle. Every call to
// Client.Subscribe produces one of these, each tied to the shared
// upstream subscription identified by sub.key.
//
// Lifecycle:
//  1. Client.Subscribe pushes a new subscriber into
//     subscription.subscribers, starts its deliverLoop goroutine.
//  2. The upstream read loop pushes notifications to subscription,
//     which fan-outs to each subscriber via enqueue.
//  3. On overflow, enqueue drops the OLDEST buffered message, not the
//     newest, so callers always see the freshest data after a burst.
//  4. Unsubscribe closes done, which unblocks deliverLoop; the
//     subscription drops this subscriber from its slice.
type subscriber struct {
	id       uint64
	handler  Handler
	subKey   string
	bufSize  int
	mu       sync.Mutex
	ring     []*types.WSMessage // ring buffer (nil-filled when empty)
	head     int                // index of oldest element
	tail     int                // next write slot
	size     int                // count of live elements
	wake     chan struct{}      // signaled on push
	done     chan struct{}      // closed on unsubscribe
	dropped  uint64             // total messages dropped for this subscriber
	logger   sdklogger.Logger
}

func newSubscriber(id uint64, h Handler, subKey string, bufSize int, logger sdklogger.Logger) *subscriber {
	if bufSize <= 0 {
		bufSize = defaultSubscriberBufferSize
	}
	return &subscriber{
		id:      id,
		handler: h,
		subKey:  subKey,
		bufSize: bufSize,
		ring:    make([]*types.WSMessage, bufSize),
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
		logger:  logger,
	}
}

// enqueue pushes msg onto the subscriber's ring, applying drop-oldest
// if the buffer is full. Thread-safe.
func (s *subscriber) enqueue(msg *types.WSMessage) {
	s.mu.Lock()
	if s.size == s.bufSize {
		// Drop-oldest: advance head, overwrite its slot.
		s.ring[s.head] = nil
		s.head = (s.head + 1) % s.bufSize
		s.size--
		atomic.AddUint64(&s.dropped, 1)
		if s.logger != nil {
			s.logger.Warn("wsinfo: subscriber dropped oldest message",
				"sub_key", s.subKey,
				"subscriber_id", s.id,
				"total_dropped", atomic.LoadUint64(&s.dropped),
			)
		}
	}
	s.ring[s.tail] = msg
	s.tail = (s.tail + 1) % s.bufSize
	s.size++
	s.mu.Unlock()

	// Non-blocking wake: deliverLoop already-awake case is fine
	// because it drains until empty on each wake.
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// deliverLoop runs on its own goroutine for the lifetime of this
// subscriber. It drains the ring and invokes the handler for every
// message.
func (s *subscriber) deliverLoop() {
	for {
		select {
		case <-s.done:
			return
		case <-s.wake:
		}
		for {
			s.mu.Lock()
			if s.size == 0 {
				s.mu.Unlock()
				break
			}
			msg := s.ring[s.head]
			s.ring[s.head] = nil
			s.head = (s.head + 1) % s.bufSize
			s.size--
			s.mu.Unlock()

			// Check done between iterations so a closed subscriber
			// stops draining promptly.
			select {
			case <-s.done:
				return
			default:
			}
			s.invokeHandler(msg)
		}
	}
}

// invokeHandler wraps the user callback in a recover so a panicking
// handler doesn't take down the deliver goroutine.
func (s *subscriber) invokeHandler(msg *types.WSMessage) {
	defer func() {
		if r := recover(); r != nil && s.logger != nil {
			s.logger.Error("wsinfo: subscriber handler panicked",
				"sub_key", s.subKey,
				"subscriber_id", s.id,
				"recovered", r,
			)
		}
	}()
	s.handler(msg)
}

// close marks the subscriber done. Safe to call multiple times.
func (s *subscriber) close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}
