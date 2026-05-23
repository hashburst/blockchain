package blockchain

import "log"

type Blockchain struct {
	Blocks       []*Block
	PendingTXs   []*Transaction
	MiningReward float64
}

func NewBlockchain() *Blockchain {
	genesis := NewBlock([]*Transaction{}, "0", 0)
	return &Blockchain{Blocks: []*Block{genesis}, PendingTXs: []*Transaction{}, MiningReward: 50.0}
}

func (bc *Blockchain) AddBlock(minerAddress string) {
	latest := bc.Blocks[len(bc.Blocks)-1]
	rewardTX := NewTransaction("System", minerAddress, bc.MiningReward)
	bc.PendingTXs = append(bc.PendingTXs, rewardTX)
	newBlock := NewBlock(bc.PendingTXs, latest.Hash, PoH(latest.ProofOfTime))
	newBlock.MineBlock()
	if bc.ValidateBlock(newBlock) {
		newBlock.Index = len(bc.Blocks)
		bc.Blocks = append(bc.Blocks, newBlock)
		bc.PendingTXs = []*Transaction{}
		log.Printf("Block #%d mined | hash: %s... | txs: %d",
			newBlock.Index, newBlock.Hash[:16], len(newBlock.Transactions))
	} else {
		log.Println("Block validation failed")
	}
}

func (bc *Blockchain) ValidateBlock(b *Block) bool {
	return b.PrevHash == bc.Blocks[len(bc.Blocks)-1].Hash && ValidatePoH(b.ProofOfTime)
}
