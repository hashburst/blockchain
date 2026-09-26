package blockchain

import (
	"testing"

	"hashburst/protocolv2"
)

func testConsensusMempoolTransaction(
	sequence uint64,
) *protocolv2.TransactionV2 {
	return protocolv2.NewTransactionV2(
		1337,
		protocolv2.TxContractDeploy,
		"0x1111111111111111111111111111111111111111",
		"",
		0,
		sequence,
		500_000,
		1,
		[]byte(`{"contract_type":"MiningPayoutRegistry"}`),
	)
}

func TestConsensusProposalStagesCurrentMempool(t *testing.T) {
	bc := &Blockchain{
		mempool: NewMempool(),
	}

	tx := testConsensusMempoolTransaction(0)

	if !bc.mempool.AddTransactionV2(tx) {
		t.Fatal("failed to add V2 transaction to mempool")
	}

	if len(bc.PendingTXsV2) != 0 {
		t.Fatalf(
			"unexpected pre-staged transactions: %d",
			len(bc.PendingTXsV2),
		)
	}

	bc.stageConsensusMempool()

	if len(bc.PendingTXsV2) != 1 {
		t.Fatalf(
			"staged V2 transactions=%d want=1",
			len(bc.PendingTXsV2),
		)
	}

	if got, want := bc.PendingTXsV2[0].HashHex(), tx.HashHex(); got != want {
		t.Fatalf(
			"staged transaction=%s want=%s",
			got,
			want,
		)
	}

	if !bc.mempool.HasTransactionV2(tx.HashHex()) {
		t.Fatal("proposal staging removed transaction from mempool")
	}
}

func TestConsensusProposalPreservesExplicitStagingWhenMempoolEmpty(
	t *testing.T,
) {
	bc := &Blockchain{
		mempool: NewMempool(),
	}

	tx := testConsensusMempoolTransaction(0)

	bc.SetPendingTransactions(
		nil,
		[]*protocolv2.TransactionV2{tx},
	)

	bc.stageConsensusMempool()

	if len(bc.PendingTXsV2) != 1 {
		t.Fatalf(
			"explicit staging was lost: got=%d want=1",
			len(bc.PendingTXsV2),
		)
	}

	if got, want := bc.PendingTXsV2[0].HashHex(), tx.HashHex(); got != want {
		t.Fatalf(
			"explicit transaction=%s want=%s",
			got,
			want,
		)
	}
}
