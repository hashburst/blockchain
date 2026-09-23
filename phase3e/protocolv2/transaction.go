package protocolv2

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"hashburst/wallet"
)

const (
	Version2 uint16 = 2
)

type TxType uint16

const (
	TxHBTTransfer TxType = iota + 1
	TxNodeRegistration
	TxValidatorRegister
	TxValidatorVote
	TxValidatorDelegate
	TxContractDeploy
	TxContractCall
	TxSystemReward
	TxSystemFeeDistribution
	TxValidatorUnbond
	TxValidatorWithdraw
	TxValidatorEvidence
)

func (t TxType) String() string {
	switch t {
	case TxHBTTransfer:
		return "HBT_TRANSFER"
	case TxNodeRegistration:
		return "NODE_REGISTRATION"
	case TxValidatorRegister:
		return "VALIDATOR_REGISTER"
	case TxValidatorVote:
		return "VALIDATOR_VOTE"
	case TxValidatorDelegate:
		return "VALIDATOR_DELEGATE"
	case TxContractDeploy:
		return "CONTRACT_DEPLOY"
	case TxContractCall:
		return "CONTRACT_CALL"
	case TxSystemReward:
		return "SYSTEM_REWARD"
	case TxSystemFeeDistribution:
		return "SYSTEM_FEE_DISTRIBUTION"
	case TxValidatorUnbond:
		return "VALIDATOR_UNBOND"
	case TxValidatorWithdraw:
		return "VALIDATOR_WITHDRAW"
	case TxValidatorEvidence:
		return "VALIDATOR_EVIDENCE"
	default:
		return fmt.Sprintf("UNKNOWN_%d", uint16(t))
	}
}

func (t TxType) IsKnown() bool {
	return t >= TxHBTTransfer && t <= TxValidatorEvidence
}

// TransactionV2 is the forward-compatible transaction envelope for HashBurst
// protocol/HVM execution. Values and fees are always expressed in native HBT
// atomic units (1 HBT = 1e8 units). Sequence is account-scoped and monotonic.
type TransactionV2 struct {
	Version      uint16 `json:"version"`
	ChainID      uint64 `json:"chain_id"`
	Type         TxType `json:"type"`
	Sender       string `json:"sender"`
	To           string `json:"to"`
	ValueUnits   int64  `json:"value_units"`
	Sequence     uint64 `json:"sequence"`
	ComputeLimit uint64 `json:"compute_limit"`
	MaxFeeUnits  int64  `json:"max_fee_units"`
	Data         []byte `json:"data,omitempty"`
	Signature    string `json:"signature,omitempty"`
	ID           string `json:"id,omitempty"`
}

func NewTransactionV2(chainID uint64, txType TxType, sender, to string, valueUnits int64, sequence, computeLimit uint64, maxFeeUnits int64, data []byte) *TransactionV2 {
	tx := &TransactionV2{
		Version:      Version2,
		ChainID:      chainID,
		Type:         txType,
		Sender:       sender,
		To:           to,
		ValueUnits:   valueUnits,
		Sequence:     sequence,
		ComputeLimit: computeLimit,
		MaxFeeUnits:  maxFeeUnits,
		Data:         append([]byte(nil), data...),
	}
	tx.ID = tx.HashHex()
	return tx
}

func (t *TransactionV2) Clone() *TransactionV2 {
	if t == nil {
		return nil
	}
	out := *t
	out.Data = append([]byte(nil), t.Data...)
	return &out
}

func (t *TransactionV2) IsSystem() bool {
	return t.Type == TxSystemReward || t.Type == TxSystemFeeDistribution
}

func (t *TransactionV2) CanonicalBytes() []byte {
	var b bytes.Buffer
	putString(&b, "HASHBURST_TX_V2")
	putUint16(&b, t.Version)
	putUint64(&b, t.ChainID)
	putUint16(&b, uint16(t.Type))
	putString(&b, strings.ToLower(t.Sender))
	putString(&b, strings.ToLower(t.To))
	putInt64(&b, t.ValueUnits)
	putUint64(&b, t.Sequence)
	putUint64(&b, t.ComputeLimit)
	putInt64(&b, t.MaxFeeUnits)
	putBytes(&b, t.Data)
	return b.Bytes()
}

func (t *TransactionV2) SigningHash() []byte {
	return wallet.Keccak256(t.CanonicalBytes())
}

func (t *TransactionV2) HashHex() string {
	return hex.EncodeToString(t.SigningHash())
}

func (t *TransactionV2) Sign(w *wallet.Wallet) error {
	if t.IsSystem() {
		return fmt.Errorf("system transaction must not be wallet-signed")
	}
	if !wallet.AddressEqual(w.Address(), t.Sender) {
		return fmt.Errorf("wallet %s does not match sender %s", w.Address(), t.Sender)
	}
	sig, err := w.Sign(t.SigningHash())
	if err != nil {
		return err
	}
	t.Signature = hex.EncodeToString(sig)
	t.ID = t.HashHex()
	return nil
}

func (t *TransactionV2) Verify(expectedChainID uint64) error {
	if t == nil {
		return fmt.Errorf("nil transaction")
	}
	if t.Version != Version2 {
		return fmt.Errorf("unsupported transaction version %d", t.Version)
	}
	if !t.Type.IsKnown() {
		return fmt.Errorf("unsupported transaction type %d", t.Type)
	}
	if t.ChainID != expectedChainID {
		return fmt.Errorf("chain id %d does not match expected %d", t.ChainID, expectedChainID)
	}
	if t.ValueUnits < 0 {
		return fmt.Errorf("negative value")
	}
	if t.MaxFeeUnits < 0 {
		return fmt.Errorf("negative max fee")
	}
	if t.ComputeLimit == 0 && (t.Type == TxContractCall || t.Type == TxContractDeploy) {
		return fmt.Errorf("contract transaction requires non-zero compute limit")
	}
	if t.ID != "" && !strings.EqualFold(strings.TrimPrefix(t.ID, "0x"), t.HashHex()) {
		return fmt.Errorf("transaction id does not match content")
	}
	if t.IsSystem() {
		if t.Signature != "" {
			return fmt.Errorf("system transaction must not contain a signature")
		}
		return nil
	}
	if !wallet.IsValidAddress(t.Sender) {
		return fmt.Errorf("invalid sender address %q", t.Sender)
	}
	if t.To != "" && !wallet.IsValidAddress(t.To) {
		return fmt.Errorf("invalid destination address %q", t.To)
	}
	if t.Type == TxHBTTransfer && t.To == "" {
		return fmt.Errorf("HBT transfer requires destination")
	}
	if t.Type == TxContractCall && t.To == "" {
		return fmt.Errorf("contract call requires destination contract")
	}
	if t.Type == TxContractDeploy && t.To != "" {
		return fmt.Errorf("contract deploy destination must be empty")
	}
	switch t.Type {
	case TxValidatorRegister:
		if t.To != "" {
			return fmt.Errorf("validator registration destination must be empty")
		}
		if t.ValueUnits <= 0 {
			return fmt.Errorf("validator registration requires bond value")
		}
	case TxValidatorUnbond, TxValidatorWithdraw, TxValidatorEvidence:
		if t.To != "" {
			return fmt.Errorf("validator protocol transaction destination must be empty")
		}
		if t.ValueUnits != 0 {
			return fmt.Errorf("validator protocol transaction value must be zero")
		}
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(t.Signature, "0x"))
	if err != nil || len(sig) != 65 {
		return fmt.Errorf("invalid 65-byte signature")
	}
	recovered, err := wallet.RecoverAddress(t.SigningHash(), sig)
	if err != nil {
		return fmt.Errorf("recover signer: %w", err)
	}
	if !wallet.AddressEqual(recovered, t.Sender) {
		return fmt.Errorf("signature belongs to %s, sender is %s", recovered, t.Sender)
	}
	return nil
}

// EncodeRaw serializes a signed TransactionV2 as UTF-8 JSON and returns a
// 0x-prefixed hex envelope. The format is intentionally HashBurst-native; it
// is not Ethereum RLP and therefore cannot be confused with eth_sendRawTransaction.
func (t *TransactionV2) EncodeRaw() (string, error) {
	b, err := json.Marshal(t)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(b), nil
}

func DecodeRawTransactionV2(raw string) (*TransactionV2, error) {
	raw = strings.TrimPrefix(strings.TrimSpace(raw), "0x")
	if raw == "" {
		return nil, fmt.Errorf("empty raw transaction")
	}
	b, err := hex.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("raw transaction is not hex: %w", err)
	}
	var tx TransactionV2
	if err := json.Unmarshal(b, &tx); err != nil {
		return nil, fmt.Errorf("raw transaction JSON: %w", err)
	}
	return &tx, nil
}

func putUint16(b *bytes.Buffer, v uint16) {
	var x [2]byte
	binary.BigEndian.PutUint16(x[:], v)
	b.Write(x[:])
}

func putUint64(b *bytes.Buffer, v uint64) {
	var x [8]byte
	binary.BigEndian.PutUint64(x[:], v)
	b.Write(x[:])
}

func putInt64(b *bytes.Buffer, v int64) {
	putUint64(b, uint64(v))
}

func putString(b *bytes.Buffer, s string) {
	putBytes(b, []byte(s))
}

func putBytes(b *bytes.Buffer, p []byte) {
	putUint64(b, uint64(len(p)))
	b.Write(p)
}
