package execution

import (
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebSocketHeadsLogsAndUnsubscribe(t *testing.T) {
	api := NewSubscriptions()
	defer api.Close()
	srv := rpc.NewServer()
	defer srv.Stop()
	if e := srv.RegisterName("eth", api.API()); e != nil {
		t.Fatal(e)
	}
	httpSrv := httptest.NewServer(srv.WebsocketHandler([]string{"*"}))
	defer httpSrv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, e := rpc.DialWebsocket(ctx, "ws"+strings.TrimPrefix(httpSrv.URL, "http"), "http://localhost")
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	var forbidden any
	if client.CallContext(ctx, &forbidden, "eth_publishFinalized", nil, nil) == nil {
		t.Fatal("publisher exposed over RPC")
	}
	heads := make(chan *types.Header, 1)
	hs, e := client.EthSubscribe(ctx, heads, "newHeads")
	if e != nil {
		t.Fatal(e)
	}
	defer hs.Unsubscribe()
	logs := make(chan *types.Log, 1)
	addr := common.HexToAddress("0x1234")
	ls, e := client.EthSubscribe(ctx, logs, "logs", LogFilter{Addresses: []common.Address{addr}})
	if e != nil {
		t.Fatal(e)
	}
	defer ls.Unsubscribe()
	header := &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(0), GasLimit: 100000, BaseFee: big.NewInt(1)}
	log := &types.Log{Address: addr, BlockHash: header.Hash(), BlockNumber: 10, Topics: []common.Hash{}, Data: []byte{}}
	if e := api.PublishFinalized(header, []*types.Log{log}); e != nil {
		t.Fatal(e)
	}
	select {
	case h := <-heads:
		if h.Number.Uint64() != 10 {
			t.Fatal("wrong head")
		}
	case <-ctx.Done():
		t.Fatal("head notification timeout")
	}
	select {
	case l := <-logs:
		if l.Address != addr {
			t.Fatal("wrong log")
		}
	case <-ctx.Done():
		t.Fatal("log notification timeout")
	case err := <-ls.Err():
		t.Fatalf("log subscription: %v", err)
	}
	ls.Unsubscribe()
	hs.Unsubscribe()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		api.mu.Lock()
		n := len(api.clients)
		api.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("subscription leaked")
}
func TestTopicFilterAndQueueBound(t *testing.T) {
	a := common.HexToHash("0x1")
	b := common.HexToHash("0x2")
	l := &types.Log{Topics: []common.Hash{a, b}}
	if !matches(l, LogFilter{Topics: [][]common.Hash{{a}, {}}}) || matches(l, LogFilter{Topics: [][]common.Hash{{b}}}) {
		t.Fatal("topic matching")
	}
	s := NewSubscriptions()
	defer s.Close()
	slow := &listener{events: make(chan event, 1), done: make(chan struct{})}
	s.clients[slow] = struct{}{}
	h := &types.Header{Number: big.NewInt(1)}
	if e := s.PublishFinalized(h, nil); e != nil {
		t.Fatal(e)
	}
	if e := s.PublishFinalized(h, nil); e != nil {
		t.Fatal(e)
	}
	select {
	case <-slow.done:
	default:
		t.Fatal("slow consumer not detached")
	}
	if len(s.clients) != 0 {
		t.Fatal("slow consumer retained")
	}
}

func TestEthereumFilterSyntax(t *testing.T) {
	var f LogFilter
	if err := f.UnmarshalJSON([]byte(`{"address":"0x0000000000000000000000000000000000001234","topics":["0x0000000000000000000000000000000000000000000000000000000000000001",null]}`)); err != nil {
		t.Fatal(err)
	}
	if len(f.Addresses) != 1 || len(f.Topics) != 2 || len(f.Topics[1]) != 0 {
		t.Fatal("filter decoding")
	}
	if f.UnmarshalJSON([]byte(`{"address":"invalid"}`)) == nil {
		t.Fatal("invalid address accepted")
	}
}
