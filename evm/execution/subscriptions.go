package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

const maxSubscriptions = 128
const subscriptionQueue = 32

// LogFilter implements positional AND / per-position OR topic matching.
// Addresses and topics are typed so malformed values fail RPC decoding.
type LogFilter struct {
	Addresses []common.Address `json:"address"`
	Topics    [][]common.Hash  `json:"topics"`
}

// UnmarshalJSON accepts the Ethereum single-address/array and topic OR syntax.
func (f *LogFilter) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*f = LogFilter{}
	for k := range fields {
		if k != "address" && k != "topics" {
			return fmt.Errorf("unsupported subscription filter field %s", k)
		}
	}
	if raw, ok := fields["address"]; ok && string(raw) != "null" {
		var one common.Address
		if json.Unmarshal(raw, &one) == nil {
			f.Addresses = []common.Address{one}
		} else if err := json.Unmarshal(raw, &f.Addresses); err != nil {
			return err
		}
	}
	if raw, ok := fields["topics"]; ok && string(raw) != "null" {
		var topics []json.RawMessage
		if err := json.Unmarshal(raw, &topics); err != nil {
			return err
		}
		for _, r := range topics {
			var values []common.Hash
			if string(r) != "null" {
				var one common.Hash
				if json.Unmarshal(r, &one) == nil {
					values = []common.Hash{one}
				} else if err := json.Unmarshal(r, &values); err != nil {
					return err
				}
			}
			f.Topics = append(f.Topics, values)
		}
	}
	return nil
}

type event struct {
	head *types.Header
	logs []*types.Log
}
type listener struct {
	events chan event
	done   chan struct{}
	heads  bool
	filter LogFilter
}

// Subscriptions is an in-process finalized-event adapter. Only the consensus
// integration may call PublishFinalized, after durable finalization. It has no
// network listener of its own and is not registered in the deployed runtime.
type Subscriptions struct {
	mu      sync.Mutex
	clients map[*listener]struct{}
}

func NewSubscriptions() *Subscriptions { return &Subscriptions{clients: make(map[*listener]struct{})} }

type SubscriptionAPI struct{ hub *Subscriptions }

// API exposes only subscription handlers, never the consensus publisher.
func (s *Subscriptions) API() *SubscriptionAPI { return &SubscriptionAPI{hub: s} }

func (api *SubscriptionAPI) NewHeads(ctx context.Context) (*rpc.Subscription, error) {
	return api.hub.subscribe(ctx, true, LogFilter{})
}
func (api *SubscriptionAPI) Logs(ctx context.Context, f LogFilter) (*rpc.Subscription, error) {
	if len(f.Topics) > 4 || len(f.Addresses) > 256 {
		return nil, fmt.Errorf("filter exceeds limits")
	}
	for _, v := range f.Topics {
		if len(v) > 256 {
			return nil, fmt.Errorf("topic filter exceeds limits")
		}
	}
	return api.hub.subscribe(ctx, false, f)
}
func (s *Subscriptions) subscribe(ctx context.Context, heads bool, f LogFilter) (*rpc.Subscription, error) {
	notifier, ok := rpc.NotifierFromContext(ctx)
	if !ok {
		return nil, rpc.ErrNotificationsUnsupported
	}
	s.mu.Lock()
	if len(s.clients) >= maxSubscriptions {
		s.mu.Unlock()
		return nil, errors.New("subscription capacity reached")
	}
	l := &listener{events: make(chan event, subscriptionQueue), done: make(chan struct{}), heads: heads, filter: f}
	s.clients[l] = struct{}{}
	s.mu.Unlock()
	sub := notifier.CreateSubscription()
	go func() {
		defer s.remove(l)
		for {
			select {
			case <-sub.Err():
				return
			case <-l.done:
				return
			case e := <-l.events:
				if l.heads {
					if notifier.Notify(sub.ID, e.head) != nil {
						return
					}
				} else {
					for _, log := range e.logs {
						if matches(log, l.filter) {
							if notifier.Notify(sub.ID, log) != nil {
								return
							}
						}
					}
				}
			}
		}
	}()
	return sub, nil
}
func (s *Subscriptions) remove(l *listener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.clients[l]; ok {
		delete(s.clients, l)
		close(l.done)
	}
}
func matches(log *types.Log, f LogFilter) bool {
	if len(f.Addresses) > 0 {
		found := false
		for _, a := range f.Addresses {
			found = found || a == log.Address
		}
		if !found {
			return false
		}
	}
	if len(f.Topics) > len(log.Topics) {
		return false
	}
	for i, choices := range f.Topics {
		if len(choices) == 0 {
			continue
		}
		found := false
		for _, topic := range choices {
			found = found || topic == log.Topics[i]
		}
		if !found {
			return false
		}
	}
	return true
}

// PublishFinalized never blocks consensus. Slow consumers are detached rather
// than accumulating unbounded memory. Callers must provide strictly ordered,
// finalized events; historical queries handle reconnect catch-up.
func (s *Subscriptions) PublishFinalized(header *types.Header, logs []*types.Log) error {
	if header == nil || header.Number == nil {
		return errors.New("missing finalized header")
	}
	copyLogs := make([]*types.Log, len(logs))
	for i, l := range logs {
		if l == nil || l.Removed || l.BlockHash != header.Hash() || l.BlockNumber != header.Number.Uint64() {
			return errors.New("log does not match finalized header")
		}
		c := *l
		c.Topics = append([]common.Hash{}, l.Topics...)
		c.Data = append([]byte(nil), l.Data...)
		copyLogs[i] = &c
	}
	e := event{head: types.CopyHeader(header), logs: copyLogs}
	s.mu.Lock()
	defer s.mu.Unlock()
	for l := range s.clients {
		select {
		case l.events <- e:
		default:
			delete(s.clients, l)
			close(l.done)
		}
	}
	return nil
}
func (s *Subscriptions) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for l := range s.clients {
		delete(s.clients, l)
		close(l.done)
	}
}
