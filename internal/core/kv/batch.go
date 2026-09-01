package kv

import (
	"context"
	"sync"
)

const defaultBatchWorkers = 8

func batchGetWithWorkers(
	ctx context.Context,
	keys []string,
	workers int,
	get func(context.Context, string) ([]byte, error),
) (map[string]Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	unique := uniqueKeys(keys)
	results := make(map[string]Result, len(unique))
	if len(unique) == 0 {
		return results, nil
	}
	if workers <= 0 {
		workers = defaultBatchWorkers
	}
	if workers > len(unique) {
		workers = len(unique)
	}

	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for key := range jobs {
				value, err := get(ctx, key)
				mu.Lock()
				results[key] = Result{Value: value, Err: err}
				mu.Unlock()
			}
		}()
	}

	for _, key := range unique {
		select {
		case jobs <- key:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func uniqueKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	unique := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	return unique
}
