package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashburst/blockchain/api"
	"github.com/hashburst/blockchain/blockchain"
	"github.com/hashburst/blockchain/config"
	"github.com/hashburst/blockchain/poh"
)

func main() {
	cfg := config.Load()
	
	bc := blockchain.New(cfg.Blockchain.Difficulty)
	poh := poh.New(cfg.PoH.IntervalMs, cfg.PoH.VerifyWindow) // Usa poh.New invece di poh.NewGenerator
	api := api.New(bc, poh, cfg)

	go poh.Generate()

	server := &http.Server{
		Addr:    cfg.Server.Address,
		Handler: api.Router(),
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on %s", cfg.Server.Address)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server...")
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}
	log.Println("Server stopped")
}
