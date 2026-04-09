package par_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"gorancid/pkg/par"
)

func TestRunAll(t *testing.T) {
	var count int64
	jobs := make([]par.Job, 10)
	for i := range jobs {
		jobs[i] = func(ctx context.Context) error {
			atomic.AddInt64(&count, 1)
			return nil
		}
	}

	results := par.Run(context.Background(), jobs, 3)
	if len(results) != 10 {
		t.Errorf("got %d results, want 10", len(results))
	}
	if count != 10 {
		t.Errorf("count = %d, want 10", count)
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error: %v", r.Err)
		}
	}
}

func TestRunErrors(t *testing.T) {
	boom := errors.New("boom")
	jobs := []par.Job{
		func(ctx context.Context) error { return nil },
		func(ctx context.Context) error { return boom },
		func(ctx context.Context) error { return nil },
	}

	results := par.Run(context.Background(), jobs, 2)
	if results[1].Err != boom {
		t.Errorf("results[1].Err = %v, want boom", results[1].Err)
	}
	if results[0].Err != nil || results[2].Err != nil {
		t.Error("expected nil errors for successful jobs")
	}
}

func TestRunRespectsParCount(t *testing.T) {
	var maxConcurrent int64
	var current int64
	jobs := make([]par.Job, 20)
	for i := range jobs {
		jobs[i] = func(ctx context.Context) error {
			c := atomic.AddInt64(&current, 1)
			if c > atomic.LoadInt64(&maxConcurrent) {
				atomic.StoreInt64(&maxConcurrent, c)
			}
			atomic.AddInt64(&current, -1)
			return nil
		}
	}

	par.Run(context.Background(), jobs, 4)
	if maxConcurrent > 4 {
		t.Errorf("max concurrent = %d, want <= 4", maxConcurrent)
	}
}