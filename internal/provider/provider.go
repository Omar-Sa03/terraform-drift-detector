package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/medom/terraform-drift-detector/internal/model"
)

type Fetcher interface {
	Name() string
	Supports(tfType string) bool
	Fetch(ctx context.Context, expected []model.Resource) ([]model.Resource, error)
}

type Registry struct {
	fetchers []Fetcher
}

func NewRegistry(fetchers ...Fetcher) *Registry {
	return &Registry{fetchers: fetchers}
}

func (r *Registry) FetchAll(ctx context.Context, expected []model.Resource) ([]model.Resource, []string, error) {
	grouped := map[string][]model.Resource{}
	var warnings []string
	for _, res := range expected {
		f := r.finder(res.Type)
		if f == nil {
			warnings = append(warnings, fmt.Sprintf("skipping unsupported type %s (%s)", res.Type, res.Address))
			continue
		}
		grouped[f.Name()] = append(grouped[f.Name()], res)
	}

	var (
		mu    sync.Mutex
		out   []model.Resource
		wg    sync.WaitGroup
		errCh = make(chan error, len(grouped))
	)
	for _, f := range r.fetchers {
		res := grouped[f.Name()]
		if len(res) == 0 {
			continue
		}
		wg.Add(1)
		go func(f Fetcher, res []model.Resource) {
			defer wg.Done()
			got, err := f.Fetch(ctx, res)
			if err != nil {
				errCh <- fmt.Errorf("%s: %w", f.Name(), err)
				return
			}
			mu.Lock()
			out = append(out, got...)
			mu.Unlock()
		}(f, res)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return out, warnings, err
		}
	}
	return out, warnings, nil
}

func (r *Registry) finder(tfType string) Fetcher {
	for _, f := range r.fetchers {
		if f.Supports(tfType) {
			return f
		}
	}
	return nil
}
