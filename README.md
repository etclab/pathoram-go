# PathORAM

Path ORAM implementation in Go with pluggable storage, encryption, and position map.

## Features

- **Pluggable backends** — Storage, Encryptor, and PositionMap interfaces for custom implementations
- **AES-256-GCM encryption** — Built-in authenticated encryption with random nonces
- **Multiple eviction strategies** — LevelByLevel, GreedyByDepth, DeterministicTwoPath
- **Constant-time mode** — For TEE deployments (SGX, TrustZone) to mitigate timing side-channels
- **Zero external dependencies** — Only Go standard library

## File Structure

```
pathoram-go/
├── config.go       # Config, EvictionStrategy, errors
├── oram.go         # PathORAM struct, New(), Access(), Read(), Write()
├── storage.go      # Storage interface + InMemoryStorage
├── encryptor.go    # Encryptor interface + AESGCMEncryptor, NoOpEncryptor
├── posmap.go       # PositionMap interface + InMemoryPositionMap
├── eviction.go     # Eviction strategies
├── constanttime.go # Constant-time operations for TEE
└── oram_test.go    # Tests and benchmarks
```

## Install

```bash
go get github.com/etclab/pathoram-go
```

## Usage

### Basic (in-memory, no encryption)

```go
import "github.com/etclab/pathoram-go"

oram, err := pathoram.NewInMemory(pathoram.Config{
    NumBlocks:  1000,  // max blocks
    BlockSize:  512,   // bytes per block
    BucketSize: 5,     // blocks per bucket (default: 5)
})

// Write (returns previous value)
data := make([]byte, 512)
prev, err := oram.Write(42, data)

// Read
data, err = oram.Read(42)

// Access: nil = read, non-nil = write
data, err = oram.Access(42, nil)        // read
prev, err = oram.Access(42, newData)    // write
```

### With encryption

```go
key := make([]byte, 32) // AES-256
rand.Read(key)

cfg := pathoram.Config{NumBlocks: 1000, BlockSize: 512, BucketSize: 4}
cfg, _ = cfg.Validate()
_, _, totalBuckets := cfg.ComputeTreeParams()

storage := pathoram.NewInMemoryStorage(totalBuckets, cfg.BucketSize, cfg.BlockSize+28) // +28 for nonce+tag
posMap := pathoram.NewInMemoryPositionMap()
enc, _ := pathoram.NewAESGCMEncryptor(key)

oram, err := pathoram.New(cfg, storage, posMap, enc)
```

### Custom backends

Implement these interfaces for custom storage, encryption, or position map:

```go
type Storage interface {
    ReadBucket(idx int) ([]Block, error)
    WriteBucket(idx int, blocks []Block) error
    NumBuckets() int
    BucketSize() int
    BlockSize() int
}

type Encryptor interface {
    Encrypt(blockID, leaf int, plaintext []byte) ([]byte, error)
    Decrypt(blockID, leaf int, ciphertext []byte) ([]byte, error)
    Overhead() int
}

type PositionMap interface {
    Get(blockID int) (leaf int, exists bool)
    Set(blockID int, leaf int)
    Size() int
}
```

## API

| Method | Description |
|--------|-------------|
| `NewInMemory(cfg)` | Create ORAM with in-memory storage, no encryption |
| `New(cfg, storage, posMap, enc)` | Create ORAM with custom backends |
| `Read(blockID) ([]byte, error)` | Read block, returns data |
| `Write(blockID, data) ([]byte, error)` | Write block, returns previous value |
| `Access(blockID, newData) ([]byte, error)` | Read if newData=nil, else write |

## Config

| Field | Description |
|-------|-------------|
| `NumBlocks` | Max blocks (required) |
| `BlockSize` | Bytes per block (required) |
| `BucketSize` | Blocks per bucket (default: 5) |
| `StashLimit` | Max stash size (default: 100) |
| `EvictionStrategy` | See below (default: LevelByLevel) |
| `ConstantTime` | Enable constant-time ops for TEE (default: false) |

## Eviction Strategies

| Strategy | Description |
|----------|-------------|
| `EvictLevelByLevel` | Baseline. Iterates levels leaf-to-root, fills slots greedily. |
| `EvictGreedyByDepth` | Places blocks at deepest possible level. Reduces stash pressure. |
| `EvictDeterministicTwoPath` | Evicts along two paths per access. Reduces stash variance. |

## Build

```bash
make build    # compile
make test     # run tests
make bench    # benchmarks
make check    # fmt + vet + test
```

## References

- Stefanov et al., [Path ORAM: An Extremely Simple Oblivious RAM Protocol](https://dl.acm.org/doi/10.1145/3177872), Journal of the ACM, 2018