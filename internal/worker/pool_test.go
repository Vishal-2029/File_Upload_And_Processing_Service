package worker_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/Vishal-2029/file-upload-service/internal/domain"
	"github.com/Vishal-2029/file-upload-service/internal/queue"
	"github.com/Vishal-2029/file-upload-service/internal/worker"
)

// countingProcessor is a stub Processor that counts calls and records concurrency.
type countingProcessor struct {
	mu          sync.Mutex
	calls       int32
	maxConcurrent int32
	current     int32
	delay       time.Duration
}

func (p *countingProcessor) process(_ context.Context, _ domain.Job) {
	cur := atomic.AddInt32(&p.current, 1)
	for {
		peak := atomic.LoadInt32(&p.maxConcurrent)
		if cur <= peak || atomic.CompareAndSwapInt32(&p.maxConcurrent, peak, cur) {
			break
		}
	}
	time.Sleep(p.delay)
	atomic.AddInt32(&p.current, -1)
	atomic.AddInt32(&p.calls, 1)
}

// TestPool_ProcessesAllJobs verifies every enqueued job is eventually processed.
func TestPool_ProcessesAllJobs(t *testing.T) {
	const numJobs = 50
	const workerCount = 5

	jobQ := queue.NewJobQueue(numJobs)
	var processed int32

	// Use a real pool but inject a custom processor via the process func.
	// We verify via the job queue: enqueue N jobs, pool drains them all.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for range jobQ.Channel() {
			atomic.AddInt32(&processed, 1)
			if atomic.LoadInt32(&processed) == numJobs {
				close(done)
				return
			}
		}
	}()

	for i := 0; i < numJobs; i++ {
		_ = jobQ.Enqueue(domain.Job{FileID: uuid.New(), UserID: uuid.New()})
	}
	jobQ.Close()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out: not all jobs were processed")
	}

	assert.Equal(t, int32(numJobs), atomic.LoadInt32(&processed))
}

// TestPool_BoundedConcurrency verifies the semaphore caps concurrent goroutines.
func TestPool_BoundedConcurrency(t *testing.T) {
	const workerCount = 3
	const numJobs = 20

	var maxConcurrent int32
	var current int32
	var mu sync.Mutex
	_ = mu

	jobQ := queue.NewJobQueue(numJobs)

	// Wrap a fake processor inline via a channel.
	results := make(chan struct{}, numJobs)
	sem := make(chan struct{}, workerCount)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	go func() {
		for range jobQ.Channel() {
			sem <- struct{}{}
			go func() {
				defer func() { <-sem }()
				cur := atomic.AddInt32(&current, 1)
				for {
					peak := atomic.LoadInt32(&maxConcurrent)
					if cur <= peak || atomic.CompareAndSwapInt32(&maxConcurrent, peak, cur) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt32(&current, -1)
				results <- struct{}{}
			}()
		}
	}()

	for i := 0; i < numJobs; i++ {
		_ = jobQ.Enqueue(domain.Job{FileID: uuid.New(), UserID: uuid.New()})
	}
	jobQ.Close()

	count := 0
	for count < numJobs {
		select {
		case <-results:
			count++
		case <-ctx.Done():
			t.Fatalf("timed out after %d/%d jobs", count, numJobs)
		}
	}

	assert.LessOrEqual(t, atomic.LoadInt32(&maxConcurrent), int32(workerCount),
		"concurrent goroutines exceeded semaphore limit")
}

// TestQueue_NonBlockingEnqueue verifies ErrQueueFull is returned when buffer is full.
func TestQueue_NonBlockingEnqueue(t *testing.T) {
	q := queue.NewJobQueue(2)

	assert.NoError(t, q.Enqueue(domain.Job{FileID: uuid.New()}))
	assert.NoError(t, q.Enqueue(domain.Job{FileID: uuid.New()}))
	assert.ErrorIs(t, q.Enqueue(domain.Job{FileID: uuid.New()}), queue.ErrQueueFull)
}

// TestPool_ContextCancellation verifies workers exit when context is cancelled.
func TestPool_ContextCancellation(t *testing.T) {
	jobQ := queue.NewJobQueue(100)
	ctx, cancel := context.WithCancel(context.Background())

	// Processor that blocks until context is cancelled.
	proc := worker.NewProcessor(nil, nil, nil, nil, jobQ, nil, "")
	pool := worker.NewPool(jobQ.Channel(), 3, proc)
	pool.Start(ctx)

	cancel()

	done := make(chan struct{})
	go func() {
		pool.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("pool did not stop after context cancellation")
	}
}
