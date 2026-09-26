package blockchain

// reward.go — la transazione di reward del mining.
//
// Prima la reward era una NewTransaction("System", miner, 50) come una tx
// qualsiasi. Ora che le tx normali richiedono una firma valida, la reward ha
// bisogno di un costruttore proprio: e' l'unica transazione che il protocollo
// genera da se', senza un wallet che la firmi. Sender == SystemSender e' la
// sentinella che dice a Verify() di non pretendere una firma.

// NewSystemReward crea la transazione di reward per il miner.
func NewSystemReward(minerAddress string, amount float64) *Transaction {
	t := &Transaction{
		Sender:   SystemSender,
		Receiver: minerAddress,
		Amount:   amount,
		Nonce:    randomNonce(), // due reward allo stesso indirizzo non collidono
	}
	t.ID = t.HashTransaction()
	return t
}
