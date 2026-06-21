package service

import (
	"log"
	"sync"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/dispatch-service/model"
)

// BatchCollector collects incoming orders and emits batches when either the
// time window (MaxWindow) expires or the minimum order threshold (MinOrders)
// is reached.
type BatchCollector struct {
	orders  chan *model.Order
	batches chan []*model.Order

	mu      sync.Mutex
	pending []*model.Order

	config  model.BatchConfig
	closeCh chan struct{}
	started bool
}

// NewBatchCollector creates a new BatchCollector with the given config.
func NewBatchCollector(cfg model.BatchConfig) *BatchCollector {
	return &BatchCollector{
		orders:  make(chan *model.Order, 100),
		batches: make(chan []*model.Order, 10),
		config:  cfg,
		closeCh: make(chan struct{}),
	}
}

// Orders returns the input channel used to enqueue orders.
func (bc *BatchCollector) Orders() chan<- *model.Order { return bc.orders }

// Batches returns the output channel that emits completed batches.
func (bc *BatchCollector) Batches() <-chan []*model.Order { return bc.batches }

// Start begins the collection loop. Safe to call multiple times.
func (bc *BatchCollector) Start() {
	bc.mu.Lock()
	if bc.started {
		bc.mu.Unlock()
		return
	}
	bc.started = true
	bc.pending = make([]*model.Order, 0, bc.config.MinOrders)
	bc.mu.Unlock()

	go bc.run()
}

// Stop terminates the collection loop and flushes remaining orders.
func (bc *BatchCollector) Stop() { close(bc.closeCh) }

func (bc *BatchCollector) run() {
	timer := time.NewTimer(bc.config.MaxWindow)
	defer timer.Stop()

	// Ensure timer starts clean
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(bc.config.MaxWindow)

	for {
		select {
		case <-bc.closeCh:
			bc.flush()
			close(bc.batches)
			return

		case o := <-bc.orders:
			bc.mu.Lock()
			bc.pending = append(bc.pending, o)
			count := len(bc.pending)
			bc.mu.Unlock()

			if count >= bc.config.MinOrders {
				bc.flush()
				resetTimer(timer, bc.config.MaxWindow)
			}

		case <-timer.C:
			bc.mu.Lock()
			hasPending := len(bc.pending) > 0
			bc.mu.Unlock()
			if hasPending {
				bc.flush()
			}
			resetTimer(timer, bc.config.MaxWindow)
		}
	}
}

func (bc *BatchCollector) flush() {
	bc.mu.Lock()
	if len(bc.pending) == 0 {
		bc.mu.Unlock()
		return
	}
	batch := make([]*model.Order, len(bc.pending))
	copy(batch, bc.pending)
	bc.pending = bc.pending[:0]
	bc.mu.Unlock()

	log.Printf("[batcher] flushing batch of %d orders", len(batch))
	bc.batches <- batch
}

func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}
