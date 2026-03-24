package queue

import (
	"errors"

	"github.com/Vishal-2029/file-upload-service/internal/domain"
)

// ErrQueueFull is returned when the job channel buffer is at capacity.
var ErrQueueFull = errors.New("job queue is full")

// JobQueue is a non-blocking buffered channel-based job queue.
type JobQueue struct {
	ch chan domain.Job
}

func NewJobQueue(size int) *JobQueue {
	return &JobQueue{ch: make(chan domain.Job, size)}
}

// Enqueue adds a job to the queue without blocking.
// Returns ErrQueueFull if the buffer is at capacity.
func (q *JobQueue) Enqueue(job domain.Job) error {
	select {
	case q.ch <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

// Channel returns the read-only job channel consumed by the worker pool.
func (q *JobQueue) Channel() <-chan domain.Job {
	return q.ch
}

// Close signals workers that no more jobs will be enqueued.
func (q *JobQueue) Close() {
	close(q.ch)
}
