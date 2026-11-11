package md

import "context"

// Source produces normalized market ticks until ctx is cancelled.
type Source interface {
	Run(ctx context.Context, out chan<- Tick) error
	Name() string
}

// Publisher delivers a tick to the message bus.
type Publisher interface {
	Publish(ctx context.Context, tick Tick) error
	Close() error
	Name() string
}
