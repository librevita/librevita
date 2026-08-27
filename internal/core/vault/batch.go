package vault

import (
	"context"
	"sync"

	"librevita.org/internal/core/crypto"
)

const defaultBatchWorkers = 8

var (
	_ crypto.BatchKeyVault       = (*BBoltVault)(nil)
	_ crypto.ConditionalKeyVault = (*BBoltVault)(nil)
	_ crypto.BatchKeyVault       = (*NATSVault)(nil)
	_ crypto.ConditionalKeyVault = (*NATSVault)(nil)
	_ crypto.BatchKeyVault       = (*EtcdVault)(nil)
	_ crypto.ConditionalKeyVault = (*EtcdVault)(nil)
	_ crypto.BatchKeyVault       = (*HashiCorpVault)(nil)
	_ crypto.ConditionalKeyVault = (*HashiCorpVault)(nil)
)

func batchGetWithWorkers(
	ctx context.Context,
	urns []string,
	workers int,
	get func(context.Context, string) ([]byte, error),
) (map[string]crypto.DEKResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	unique := uniqueURNs(urns)
	results := make(map[string]crypto.DEKResult, len(unique))
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
			for urn := range jobs {
				value, err := get(ctx, urn)
				mu.Lock()
				results[urn] = crypto.DEKResult{
					EncryptedDEK: value,
					Err:          err,
				}
				mu.Unlock()
			}
		}()
	}

	for _, urn := range unique {
		select {
		case jobs <- urn:
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

func uniqueURNs(urns []string) []string {
	seen := make(map[string]struct{}, len(urns))
	unique := make([]string, 0, len(urns))
	for _, urn := range urns {
		if _, ok := seen[urn]; ok {
			continue
		}
		seen[urn] = struct{}{}
		unique = append(unique, urn)
	}
	return unique
}
