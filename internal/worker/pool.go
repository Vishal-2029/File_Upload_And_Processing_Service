package worker

import (
	"context"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/Vishal-2029/file-upload-service/internal/domain"
)

// Pool is a bounded worker pool with a semaphore for concurrency control.
//
// Architecture (portfolio centrepiece):
//
//	workerCount dispatcher goroutines pull jobs from the shared channel.
//	Each dispatcher acquires a semaphore slot before spawning a processing
//	goroutine, ensuring at most workerCount jobs execute concurrently.
//	Semaphore is released when the job finishes (or panics).
type Pool struct {
	jobCh       <-chan domain.Job
	sem         chan struct{}
	workerCount int
	processor   *Processor
	wg          sync.WaitGroup
}

func NewPool(jobCh <-chan domain.Job, workerCount int, processor *Processor) *Pool {
	return &Pool{
		jobCh:       jobCh,
		sem:         make(chan struct{}, workerCount),
		workerCount: workerCount,
		processor:   processor,
	}
}

// Start launches workerCount dispatcher goroutines and returns immediately.
// Pass a cancellable context; closing the job channel also drains workers.
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.dispatch(ctx)
	}
	log.Info().Int("workers", p.workerCount).Msg("worker pool started")
}

// Wait blocks until all dispatcher goroutines have exited.
func (p *Pool) Wait() {
	p.wg.Wait()
	log.Info().Msg("worker pool drained")
}

func (p *Pool) dispatch(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-p.jobCh:
			if !ok {
				// channel closed — drain complete
				return
			}

			// Acquire semaphore slot before spawning the processing goroutine.
			// This caps concurrency even if many dispatcher goroutines are ready.
			select {
			case p.sem <- struct{}{}:
			case <-ctx.Done():
				return
			}

			go func(j domain.Job) {
				defer func() {
					<-p.sem // release slot
					if r := recover(); r != nil {
						log.Error().Interface("panic", r).Str("file_id", j.FileID.String()).Msg("worker panic recovered")
					}
				}()
				p.processor.Process(context.Background(), j)
			}(job)
		}
	}
}
