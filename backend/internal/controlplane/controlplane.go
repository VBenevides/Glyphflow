package controlplane

import (
	"context"
	"sync"
)

type Component func(context.Context) error

type Plane struct {
	components []Component
	stopOnce   sync.Once
}

func New(components ...Component) *Plane { return &Plane{components: components} }

func (p *Plane) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, len(p.components))
	for _, component := range p.components {
		wg.Add(1)
		go func(run Component) {
			defer wg.Done()
			if err := run(ctx); err != nil {
				errs <- err
				cancel()
			}
		}(component)
	}
	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func (p *Plane) Stop() { p.stopOnce.Do(func() {}) }
