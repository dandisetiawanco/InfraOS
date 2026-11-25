package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/dandisetiawanco/InfraOS/pkg/telemetry"
)

type Job struct {
	Type string
	Run  func(ctx context.Context) error
}

type Queue interface {
	Enqueue(job Job) error
	Start(ctx context.Context)
	Shutdown(ctx context.Context) error
}

type InProcessQueue struct {
	jobs    chan Job
	workers int
	logger  *slog.Logger
	metrics *telemetry.Metrics
	wg      sync.WaitGroup
	mu      sync.Mutex
	closed  bool
}

func NewInProcess(bufferSize, workers int, logger *slog.Logger, metrics *telemetry.Metrics) *InProcessQueue {
	if bufferSize <= 0 {
		bufferSize = 64
	}
	if workers <= 0 {
		workers = 4
	}
	return &InProcessQueue{
		jobs:    make(chan Job, bufferSize),
		workers: workers,
		logger:  logger,
		metrics: metrics,
	}
}

func (q *InProcessQueue) Enqueue(job Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return errors.New("worker queue is shutting down")
	}
	select {
	case q.jobs <- job:
		if q.metrics != nil {
			q.metrics.DeploymentQueueLen.Set(float64(len(q.jobs)))
		}
		return nil
	default:
		return errors.New("worker queue capacity reached")
	}
}

func (q *InProcessQueue) Start(ctx context.Context) {
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go func(workerID int) {
			defer q.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-q.jobs:
					if !ok {
						return
					}
					if q.metrics != nil {
						q.metrics.DeploymentQueueLen.Set(float64(len(q.jobs)))
					}
					start := time.Now()
					err := job.Run(ctx)
					outcome := "success"
					if err != nil {
						outcome = "error"
						q.logger.Error("background job failed", "worker", workerID, "job_type", job.Type, "error", err)
					}
					if q.metrics != nil {
						q.metrics.WorkerDuration.WithLabelValues(job.Type, outcome).Observe(time.Since(start).Seconds())
					}
				}
			}
		}(i + 1)
	}
}

func (q *InProcessQueue) Shutdown(ctx context.Context) error {
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		close(q.jobs)
	}
	q.mu.Unlock()

	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
