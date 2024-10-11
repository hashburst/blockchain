package blockchain

import (
    "time"
    "math/big"
    "fmt"
    "golang.org/x/crypto/blake2b"
    // Import Mempool in the package blockchain
    "path/to/mempool"  // Update the path of mempool.go
)
/*import (
    "time"
    "math/big"
    "fmt"
    "golang.org/x/crypto/blake2b"
)
*/
var mempool = mempool.NewMempool()

// Set the target block time in seconds (e.g., 10 minutes)
const TargetBlockTime = 600 

// DifficultyAdjustmentWindow is the number of blocks after which difficulty is adjusted
const DifficultyAdjustmentWindow = 10 

// Last block's timestamp and current difficulty
var lastBlockTime time.Time
var currentDifficulty *big.Int = big.NewInt(1) // Initial difficulty

// Function to dynamically adjust difficulty
func AdjustDifficulty(lastBlockTimestamp time.Time, blockHeight int) *big.Int {
    if blockHeight%DifficultyAdjustmentWindow != 0 {
        // No adjustment needed
        return currentDifficulty
    }

    // Time difference between the last `DifficultyAdjustmentWindow` blocks
    timeTaken := time.Since(lastBlockTimestamp).Seconds()
    expectedTime := TargetBlockTime * DifficultyAdjustmentWindow

    if timeTaken < expectedTime {
        // If blocks are being mined too fast, increase the difficulty
        currentDifficulty = new(big.Int).Mul(currentDifficulty, big.NewInt(2)) // Double difficulty
        fmt.Println("Increased difficulty due to fast mining")
    } else if timeTaken > expectedTime {
        // If blocks are being mined too slowly, reduce the difficulty
        currentDifficulty = new(big.Int).Div(currentDifficulty, big.NewInt(2)) // Halve difficulty
        fmt.Println("Decreased difficulty due to slow mining")
    }
    return currentDifficulty
}
/*
func MineBlock(block *Block) {
    for {
        // Check if the hash of the block satisfies the current difficulty
        if IsValidHash(block.Hash(), currentDifficulty) {
            // Valid block found
            fmt.Println("Block mined: ", block.Hash())
            break
        }
        // Increment nonce and try again
        block.Nonce++
    }

    // Adjust difficulty after mining each block
    block.Difficulty = AdjustDifficulty(block.Timestamp, block.Height)
}
*/
func MineBlock(block *Block) {
    transactions := mempool.GetTransactions() // Get all transactions from Mempool
    block.Transactions = transactions          // Add transactions to new block

    for {
        // Check if the hash of the block satisfies the current difficulty
        if IsValidHash(block.Hash(), currentDifficulty) {
            fmt.Println("Block mined: ", block.Hash())
            break
        }
        // Increment nonce and try again
        block.Nonce++
    }

    // Adjust difficulty after mining each block
    block.Difficulty = AdjustDifficulty(block.Timestamp, block.Height)

    // Remove all transactions from Mempool after the block has mined
    for _, tx := range transactions {
        mempool.RemoveTransaction(tx.ID)
    }
}

// Use Blake2 hashing function for more energy-efficient mining
func HashBlock(data []byte) []byte {
    hash := blake2b.Sum256(data)
    return hash[:]
}

func IsValidHash(hash []byte, difficulty *big.Int) bool {
    target := new(big.Int).Lsh(big.NewInt(1), uint(256-difficulty.BitLen()))
    hashInt := new(big.Int).SetBytes(hash)
    return hashInt.Cmp(target) < 0
}

type Staker struct {
    Address string
    Stake   *big.Int
}

var stakers []Staker

// Function to reward stakers based on their stake
func RewardStakers(blockReward *big.Int) {
    totalStake := big.NewInt(0)
    for _, staker := range stakers {
        totalStake.Add(totalStake, staker.Stake)
    }

    for _, staker := range stakers {
        reward := new(big.Int).Div(new(big.Int).Mul(staker.Stake, blockReward), totalStake)
        fmt.Printf("Rewarding staker %s with %s tokens\n", staker.Address, reward.String())
    }
}

// Function to handle both mining and staking rewards
func DistributeRewards(miner string, blockReward *big.Int) {
    minerReward := new(big.Int).Div(blockReward, big.NewInt(2)) // 50% for miner
    fmt.Printf("Miner %s receives %s tokens\n", miner, minerReward.String())
    
    // Distribute the other 50% to stakers
    RewardStakers(new(big.Int).Div(blockReward, big.NewInt(2)))
}
/*
func MineBlockWithPoS(miner string, block *Block, blockReward *big.Int) {
    // Perform mining using PoW
    MineBlock(block)

    // Distribute rewards to miner and stakers using PoW/PoS hybrid model
    DistributeRewards(miner, blockReward)
}
*/
func MineBlockWithPoS(miner string, block *Block, blockReward *big.Int) {
    // Get transactions from Mempool before to start mining
    transactions := mempool.GetTransactions()
    block.Transactions = transactions

    // Perform mining using PoW
    MineBlock(block)

    // Remove transactions in the mined block from Mempool
    for _, tx := range transactions {
        mempool.RemoveTransaction(tx.ID)
    }

    // Distribute rewards to miner and stakers using PoW/PoS hybrid model
    DistributeRewards(miner, blockReward)
}
