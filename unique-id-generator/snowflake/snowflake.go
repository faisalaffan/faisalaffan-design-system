package snowflake

import (
	"fmt"
	"sync"
	"time"
)

const (
	epoch         = 1704067200000 // 2024-01-01T00:00:00Z in ms
	workerBits    = 10
	sequenceBits  = 12
	maxWorker     = 1<<workerBits - 1  // 1023
	maxSequence   = 1<<sequenceBits - 1 // 4095

	timestampShift = workerBits + sequenceBits // 22
	workerShift    = sequenceBits              // 12
)

type Generator struct {
	mu        sync.Mutex
	workerID  int64
	sequence  int64
	lastStamp int64
}

func New(workerID int64) (*Generator, error) {
	if workerID < 0 || workerID > maxWorker {
		return nil, fmt.Errorf("worker ID must be between 0 and %d", maxWorker)
	}
	return &Generator{workerID: workerID}, nil
}

func (g *Generator) Next() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli()
	if now < g.lastStamp {
		return 0, fmt.Errorf("clock moved backwards, refusing to generate ID")
	}

	if now == g.lastStamp {
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			for now <= g.lastStamp {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		g.sequence = 0
	}

	g.lastStamp = now

	id := (now-epoch)<<timestampShift |
		g.workerID<<workerShift |
		g.sequence

	return id, nil
}
