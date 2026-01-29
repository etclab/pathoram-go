# PathORAM

Path ORAM implementation in Go.

## Install

```bash
go get github.com/etclab/pathoram-go
```

## Usage

```go
import "github.com/etclab/pathoram-go"

// Create ORAM instance
oram, err := pathoram.NewPathORAM(pathoram.Config{
    NumBlocks:  1000,  // max blocks
    BlockSize:  512,   // bytes per block
    BucketSize: 5,     // blocks per bucket (default: 5)
    StashLimit: 100,   // max stash size (default: 100)
})

// Write
data := make([]byte, 512)
err = oram.Write(42, data)

// Read
data, err = oram.Read(42)

// Unified access
data, err = oram.Access(pathoram.OpWrite, 42, data)
data, err = oram.Access(pathoram.OpRead, 42, nil)
```

## API

| Method | Description |
|--------|-------------|
| `NewPathORAM(cfg)` | Create new ORAM instance |
| `Read(blockID)` | Read block data |
| `Write(blockID, data)` | Write block data |
| `Access(op, blockID, data)` | Unified read/write |
| `Size()` | Number of allocated blocks |
| `Capacity()` | Max blocks (from config) |

## Build

```bash
make build    # compile
make test     # run tests
make bench    # benchmarks
make check    # fmt + vet + test
```

## References

- Stefanov et al., "Path ORAM: An Extremely Simple Oblivious RAM Protocol" (CCS 2013)
