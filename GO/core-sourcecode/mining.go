// internal/mining/mining.go

package mining

import (
    "crypto/rand"
    "fmt"
    "golang.org/x/crypto/blake2b"
    "math/big"
    "time"
)

const (
    TargetBlockTime           = 600            // Target block time in seconds (10 minutes)
    DifficultyAdjustmentWindow = 10            // Adjust difficulty after every 10 blocks
    InitialDifficulty          = 16            // Starting difficulty level
)

var (
    currentDifficulty = big.NewInt(InitialDifficulty) // Set initial difficulty
    lastBlockTime     time.Time                        // Time when the last block was mined
    blockHeight       int                              // Current block height
)

// Block structure to represent a mined block
type Block struct {
    Height     int
    Nonce      int64
    Timestamp  time.Time
    Hash       []byte
    Difficulty *big.Int
}

// StartMining initiates the mining process
func StartMining(miner string) {
    fmt.Println("Starting APoW mining...")
    lastBlockTime = time.Now()

    for {
        block := &Block{
            Height:     blockHeight + 1,
            Nonce:      generateNonce(),
            Timestamp:  time.Now(),
            Difficulty: currentDifficulty,
        }

        mineBlock(block)
        adjustDifficulty()
        distributeRewards(miner, block)

        blockHeight++
        lastBlockTime = time.Now()
        fmt.Printf("New block mined: Height=%d, Hash=%x, Difficulty=%d\n", block.Height, block.Hash, block.Difficulty)
    }
}

// mineBlock attempts to find a valid block by solving the PoW
func mineBlock(block *Block) {
    for {
        // Hash the block data
        blockHash := hashBlock(block)
        if isValidHash(blockHash, block.Difficulty) {
            block.Hash = blockHash
            break
        }
        // Increment nonce and try again
        block.Nonce++
    }
}

// hashBlock generates a hash for the block using Blake2b for energy-efficient hashing
func hashBlock(block *Block) []byte {
    data := fmt.Sprintf("%d%d%d", block.Height, block.Nonce, block.Timestamp.Unix())
    hash := blake2b.Sum256([]byte(data))
    return hash[:]
}

// isValidHash checks if the block hash meets the current difficulty target
func isValidHash(hash []byte, difficulty *big.Int) bool {
    target := new(big.Int).Lsh(big.NewInt(1), uint(256-difficulty.BitLen())) // Calculate target based on difficulty
    hashInt := new(big.Int).SetBytes(hash)
    return hashInt.Cmp(target) < 0
}

// adjustDifficulty recalculates the difficulty based on the time taken to mine blocks
func adjustDifficulty() {
    if blockHeight%DifficultyAdjustmentWindow != 0 {
        return
    }

    timeTaken := time.Since(lastBlockTime).Seconds()
    expectedTime := TargetBlockTime * DifficultyAdjustmentWindow

    if timeTaken < expectedTime {
        currentDifficulty = new(big.Int).Mul(currentDifficulty, big.NewInt(2)) // Increase difficulty
        fmt.Println("Increased difficulty")
    } else if timeTaken > expectedTime {
        currentDifficulty = new(big.Int).Div(currentDifficulty, big.NewInt(2)) // Decrease difficulty
        fmt.Println("Decreased difficulty")
    }
}

// distributeRewards splits rewards between the miner and stakers (PoS component)
func distributeRewards(miner string, block *Block) {
    blockReward := big.NewInt(50) // Example block reward

    // Miner gets half the reward
    minerReward := new(big.Int).Div(blockReward, big.NewInt(2))
    fmt.Printf("Miner %s rewarded %s tokens\n", miner, minerReward.String())

    // Stakers share the other half (in PoS hybrid)
    stakerReward := new(big.Int).Div(blockReward, big.NewInt(2))
    rewardStakers(stakerReward)
}

// rewardStakers distributes rewards to stakeholders
func rewardStakers(stakerReward *big.Int) {
    // This can be based on stakes (PoS component)
    stakers := []string{"staker1", "staker2"} // Example stakers
    for _, staker := range stakers {
        fmt.Printf("Rewarding staker %s with %s tokens\n", staker, stakerReward.String())
    }
}

// generateNonce generates a random nonce for the block
func generateNonce() int64 {
    n, err := rand.Int(rand.Reader, big.NewInt(1<<63))
    if err != nil {
        panic(err)
    }
    return n.Int64()
}
