package github

import (
	"context"
	"sync"
)

type ScanInput struct {
	URL     string
	Dates   []string
	Targets map[string][]string
}

type ScanError struct {
	URL string
	Err string
}

const maxConcurrency = 6

func (c *Client) Collect(ctx context.Context, items []ScanInput, maxDepth int) (map[string]*Scan, []ScanError) {
	scans := map[string]*Scan{}
	var errs []ScanError
	visited := map[string]bool{}
	var mu sync.Mutex

	type task struct {
		url     string
		targets map[string][]string
		depth   int
	}

	frontier := make([]task, 0, len(items))
	for _, it := range items {
		url := NormalizeURL(it.URL)
		if url == "" || visited[url] {
			continue
		}
		visited[url] = true
		frontier = append(frontier, task{url: url, targets: it.Targets, depth: 0})
	}

	for len(frontier) > 0 {
		sem := make(chan struct{}, maxConcurrency)
		var wg sync.WaitGroup
		type result struct {
			task task
			scan *Scan
			err  string
		}
		results := make([]result, len(frontier))

		for i, t := range frontier {
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, t task) {
				defer wg.Done()
				defer func() { <-sem }()
				scan, err := c.fetchScan(ctx, t.url)
				if err != nil {
					results[i] = result{task: t, err: err.Error()}
					return
				}
				results[i] = result{task: t, scan: scan}
			}(i, t)
		}
		wg.Wait()

		var next []task
		for _, r := range results {
			if r.err != "" {
				mu.Lock()
				errs = append(errs, ScanError{URL: r.task.url, Err: r.err})
				mu.Unlock()
				continue
			}
			scans[r.task.url] = r.scan
			if r.task.depth >= maxDepth || !needsRelatedScan(r.scan, r.task.targets) {
				continue
			}
			for _, linked := range r.scan.LinkedURLs {
				if visited[linked] {
					continue
				}
				visited[linked] = true
				next = append(next, task{url: linked, targets: r.task.targets, depth: r.task.depth + 1})
			}
		}
		frontier = next
	}

	return scans, errs
}

func (c *Client) fetchScan(ctx context.Context, url string) (*Scan, error) {
	ref, _, ok := RefFromURL(url)
	if !ok {
		return &Scan{URL: url}, nil
	}
	return c.FetchNode(ctx, ref)
}

func needsRelatedScan(scan *Scan, targets map[string][]string) bool {
	for date, statuses := range targets {
		onDate := eventsOnDate(scan.Events, date)
		hasEnd := false
		for _, s := range statuses {
			if HasValidEnd(onDate, s) {
				hasEnd = true
				break
			}
		}
		if !hasEnd {
			return true
		}
	}
	return false
}

func eventsOnDate(events []Event, date string) []Event {
	var out []Event
	for _, e := range events {
		if JakartaDate(e.DateTime) == date {
			out = append(out, e)
		}
	}
	return out
}
