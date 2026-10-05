package blockchain

import (
	"context"
	"io"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// Exercise actual encrypted streams and reconnects through the selected
// libp2p dependency graph. These tests do not change production listeners.
func TestTransportDependencyCompatibility(t *testing.T) {
	for _, address := range []string{"/ip4/127.0.0.1/tcp/0", "/ip4/127.0.0.1/udp/0/quic-v1", "/ip4/127.0.0.1/udp/0/quic-v1/webtransport"} {
		t.Run(address, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			server, err := libp2p.New(libp2p.ListenAddrStrings(address))
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			client, err := libp2p.New(libp2p.NoListenAddrs)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			id := protocol.ID("/hashburst/transport-compatibility/1")
			server.SetStreamHandler(id, func(s network.Stream) {
				defer s.Close()
				_ = s.SetDeadline(time.Now().Add(10 * time.Second))
				b := make([]byte, 4)
				if _, err := io.ReadFull(s, b); err != nil {
					return
				}
				_, _ = s.Write(b)
			})
			for attempt := 0; attempt < 2; attempt++ {
				if err := client.Connect(ctx, peer.AddrInfo{ID: server.ID(), Addrs: server.Addrs()}); err != nil {
					t.Fatal(err)
				}
				s, err := client.NewStream(ctx, server.ID(), id)
				if err != nil {
					t.Fatal(err)
				}
				_ = s.SetDeadline(time.Now().Add(10 * time.Second))
				if _, err = s.Write([]byte("HVM!")); err != nil {
					s.Reset()
					t.Fatal(err)
				}
				b := make([]byte, 4)
				_, err = io.ReadFull(s, b)
				s.Close()
				if err != nil {
					t.Fatal(err)
				}
				if string(b) != "HVM!" {
					t.Fatalf("stream payload differs: %q", b)
				}
				if err := client.Network().ClosePeer(server.ID()); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
