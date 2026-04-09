package par

import (
	"context"
	"sync"
)

// Job is a unit of work executed by the pool.
type Job func(ctx context.Context) error

// Result holds the outcome of a single job.
type Result struct {
	Index int
	Err   error
}

// Run executes jobs concurrently with at most concurrency goroutines running at once.
// Results are returned indexed to match the input jobs slice.
// All jobs run to completion (or context cancellation) before returning.
func Run(ctx context.Context, jobs []Job, concurrency int) []Result {
	if concurrency <= 0 {
		concurrency = 5
	}
	results := make([]Result, len(jobs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, job := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, j Job) {
			defer wg.Done()
			defer func() { <-sem }()
			results[idx] = Result{Index: idx, Err: j(ctx)}
		}(i, job)
	}
	wg.Wait()
	return results
}