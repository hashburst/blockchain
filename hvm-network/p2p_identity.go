package main

// p2p_identity.go — Gestione identità P2P persistente
//
// Problema risolto: libp2p genera un nuovo peer ID ad ogni avvio
// se non trova una chiave su disco. Il peer ID deve essere stabile
// perché viene registrato nella blockchain come identità permanente del nodo.
//
// Soluzione: salva la chiave privata Ed25519 libp2p su disco cifrato.
// Il peer ID derivato da quella chiave è deterministico e permanente.

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

const defaultKeyPath = "/var/lib/hashburst/node_p2p.key"

// P2PIdentity contiene la chiave privata e il peer ID del nodo
type P2PIdentity struct {
	PrivKey crypto.PrivKey
	PeerID  peer.ID
	KeyPath string
}

// LoadOrCreateP2PIdentity carica la chiave da disco o ne crea una nuova.
// La chiave viene salvata in formato JSON nel path specificato.
// Questa funzione garantisce che il peer ID sia stabile tra i riavvii.
func LoadOrCreateP2PIdentity(keyPath string) (*P2PIdentity, error) {
	if keyPath == "" {
		keyPath = defaultKeyPath
	}

	// Assicura che la directory esista
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, fmt.Errorf("create key dir: %w", err)
	}

	var privKey crypto.PrivKey

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		// Prima accensione: genera nuova chiave Ed25519
		log.Printf("Generating new P2P identity key: %s", keyPath)
		privKey, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}

		// Serializza e salva
		keyBytes, err := crypto.MarshalPrivateKey(privKey)
		if err != nil {
			return nil, fmt.Errorf("marshal key: %w", err)
		}

		type keyFile struct {
			Type string `json:"type"`
			Key  []byte `json:"key"`
		}
		data, _ := json.Marshal(keyFile{Type: "Ed25519", Key: keyBytes})
		if err := os.WriteFile(keyPath, data, 0600); err != nil {
			return nil, fmt.Errorf("save key: %w", err)
		}
		log.Printf("P2P identity key saved: %s", keyPath)

	} else {
		// Carica chiave esistente
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("read key: %w", err)
		}

		type keyFile struct {
			Type string `json:"type"`
			Key  []byte `json:"key"`
		}
		var kf keyFile
		if err := json.Unmarshal(data, &kf); err != nil {
			return nil, fmt.Errorf("parse key file: %w", err)
		}

		privKey, err = crypto.UnmarshalPrivateKey(kf.Key)
		if err != nil {
			return nil, fmt.Errorf("unmarshal key: %w", err)
		}
		log.Printf("P2P identity loaded from: %s", keyPath)
	}

	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("derive peer ID: %w", err)
	}

	log.Printf("P2P Peer ID: %s", peerID)

	return &P2PIdentity{
		PrivKey: privKey,
		PeerID:  peerID,
		KeyPath: keyPath,
	}, nil
}
