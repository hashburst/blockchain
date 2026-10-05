package main

import (
	"hashburst/blockchain"
	"net/http/httptest"
	"testing"
)

func TestArchiveRejectsAllWrites(t *testing.T) {
	blocks := make([]*blockchain.Block, 11)
	for i := range blocks {
		blocks[i] = &blockchain.Block{Index: i}
	}
	h := handler(blocks, blockchain.NewState())
	for _, path := range []string{"/api/health", "/api/transactions", "/rpc", "/api/mine", "/api/register", "/api/import"} {
		for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
			r := httptest.NewRequest(method, path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 405 {
				t.Fatalf("write allowed %s %s", method, path)
			}
		}
	}
	for _, path := range []string{"/api/health", "/api/blocks", "/api/balances", "/api/transactions"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatal(path, w.Code)
		}
	}
	for _, path := range []string{"/rpc", "/api/mine", "/api/register", "/api/import"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal("unexpected route", path)
		}
	}
}
