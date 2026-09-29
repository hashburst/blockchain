package blockchain

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
)

// APoWHandler is miner control-plane ingress, loopback only. It is deliberately
// absent from the Ethereum/public gateway allowlist. Miners may use SSH tunnels.
func (bc *Blockchain) APoWHandler(broadcast func(APoWProof) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			http.Error(w, "loopback required", 403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		switch r.Method {
		case "GET":
			job, err := bc.APoWJob()
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			_ = json.NewEncoder(w).Encode(job)
		case "POST":
			var p APoWProof
			dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&p); err != nil {
				http.Error(w, "invalid work proof", 400)
				return
			}
			var extra any
			if dec.Decode(&extra) != io.EOF {
				http.Error(w, "trailing JSON", 400)
				return
			}
			if err := bc.SubmitAPoW(p); err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			if broadcast != nil {
				if err := broadcast(p); err != nil {
					http.Error(w, err.Error(), 503)
					return
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"accepted": true})
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", 405)
		}
	})
}
