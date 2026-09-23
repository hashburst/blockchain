package testnet

import (
	"context"
	"encoding/json"
	"fmt"
	"hashburst/blockchain"
	"net"
	"net/http"
	"sync"
	"time"
)

// Run owns listeners and goroutines. No administrative HTTP endpoints exist.
func Run(parent context.Context, s *State) error {
	c := s.Config
	listener, e := net.Listen("tcp", c.RPCListen)
	if e != nil {
		return e
	}
	defer listener.Close()
	mp := blockchain.NewMempool()
	s.Chain.SetMempool(mp)
	p, e := blockchain.NewP2PNodeWithListenIP(s.Chain, mp, c.P2PListenIP, c.P2PPort, s.Key)
	if e != nil {
		return e
	}
	defer p.Host.Close()
	syncer := blockchain.NewSyncer(s.Chain, mp, p.Host)
	s.Chain.SetSyncer(syncer)
	validator := ""
	if s.Signer != nil {
		validator = c.ValidatorID
	}
	// Avoid a typed nil signer interface for observers.
	var reactor *blockchain.ConsensusReactor
	if s.Signer != nil {
		reactor, e = blockchain.NewConsensusReactor(s.Chain, validator, s.Signer, nil)
	} else {
		reactor, e = blockchain.NewConsensusReactor(s.Chain, "", nil, nil)
	}
	if e != nil {
		return e
	}
	network, e := blockchain.NewLibp2pConsensusNetwork(p.Host, reactor, c.Protocol.ConsensusNetwork)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	rpc := blockchain.NewRPCHandler(s.Chain, mp, int64(c.Protocol.ChainID))
	rpc.SetV2Broadcaster(syncer)
	mux := http.NewServeMux()
	mux.Handle("/rpc", http.MaxBytesHandler(rpc, int64(c.Protocol.ConsensusNetwork.MaxMessageBytes)))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "GET required", 405)
			return
		}
		status, fresh := reactor.TryStatus()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "network": "testnet", "node_id": c.NodeID, "role": c.Role, "chain_id": c.Protocol.ChainID, "config_digest": c.Pin(), "height": s.Chain.Height(), "finalized_height": s.Chain.FinalizedHeight(), "peer_id": p.Host.ID().String(), "peer_count": len(p.Host.Network().Peers()), "reactor_running": reactor.Running(), "reactor_status_fresh": fresh, "reactor": status, "transport": network.Status()})
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if e := server.Serve(listener); e != nil && e != http.ErrServerClosed {
			errs <- e
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			for _, addr := range c.Bootnodes {
				if ctx.Err() != nil {
					return
				}
				cc, stop := context.WithTimeout(ctx, 2*time.Second)
				_ = p.Connect(cc, addr)
				stop()
			}
			syncer.HelloAll()
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !c.Protocol.ConsensusEnabledAt(s.Chain.Height() + 1) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
		if e := reactor.Run(ctx); e != nil && ctx.Err() == nil {
			errs <- fmt.Errorf("reactor: %w", e)
		}
	}()
	var result error
	select {
	case <-parent.Done():
	case result = <-errs:
	}
	cancel()
	_ = server.Close()
	_ = p.Host.Close()
	wg.Wait()
	return result
}
