package main

import (
	"hashburst/internal/evmgateway"
	"log"
	"net/http"
	"time"
)

func main() {
	s := &http.Server{Addr: "127.0.0.1:18011", Handler: evmgateway.New("http://127.0.0.1:18009/evm", "ws://127.0.0.1:18009/evm/ws", "https://blockchainapi.one"), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	log.Fatal(s.ListenAndServe())
}
