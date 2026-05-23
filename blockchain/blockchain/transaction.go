package blockchain

type Transaction struct {
	Type       string      `json:"type"`
	UserID     string      `json:"userId"`
	BlockIdSig string      `json:"blockIdSig"`
	Document   *Document   `json:"document,omitempty"`
	UserData   *UserData   `json:"userData,omitempty"`
	Wallet     *WalletData `json:"wallet,omitempty"`
	Timestamp  int64       `json:"timestamp"`
	Data       interface{} `json:"data"` // Aggiunto campo Data
}

type Document struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

type UserData struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"oldValue,omitempty"`
	NewValue interface{} `json:"newValue"`
}

type WalletData struct {
	Address   string `json:"address"`
	Signature string `json:"signature"`
}
