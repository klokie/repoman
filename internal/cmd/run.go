package cmd

import (
	"fmt"
	"sync"

	"github.com/klokie/repoman/internal/manifest"
)

// result is the outcome of one repo-level operation.
type result struct {
	Repo    manifest.Repo
	Message string
	Err     error
	Skipped bool
}

// forEachRepo runs fn over repos with at most jobs running concurrently,
// printing each line as it finishes. Results keep manifest order.
func forEachRepo(repos []manifest.Repo, jobs int, fn func(manifest.Repo) result) []result {
	if jobs < 1 {
		jobs = 1
	}
	results := make([]result, len(repos))
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, repo := range repos {
		wg.Add(1)
		go func(i int, repo manifest.Repo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := fn(repo)
			r.Repo = repo
			results[i] = r

			mu.Lock()
			defer mu.Unlock()
			switch {
			case r.Err != nil:
				fmt.Printf("  %s %-28s %s\n", red("✗"), repo.Name, red(r.Err.Error()))
			case r.Skipped:
				fmt.Printf("  %s %-28s %s\n", dim("·"), repo.Name, dim(r.Message))
			default:
				fmt.Printf("  %s %-28s %s\n", green("✓"), repo.Name, r.Message)
			}
		}(i, repo)
	}
	wg.Wait()
	return results
}

func tally(results []result) (ok, skipped, failed int) {
	for _, r := range results {
		switch {
		case r.Err != nil:
			failed++
		case r.Skipped:
			skipped++
		default:
			ok++
		}
	}
	return
}
