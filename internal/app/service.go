package app

import "context"

// Service defines the common contract for long-running background components
// within the application lifecycle (e.g. HTTP server, periodic cron jobs, bot pollers).
//
// Implementations of Start must block until ctx is done or until an unrecoverable
// error occurs. When ctx is canceled, Start must perform necessary graceful shutdown
// procedures and return nil (or an error describing shutdown failure).
type Service interface {
	Start(ctx context.Context) error
}

// ServiceFunc is an adapter to allow the use of ordinary functions as Service implementations.
type ServiceFunc func(ctx context.Context) error

// Start calls f(ctx).
func (f ServiceFunc) Start(ctx context.Context) error {
	return f(ctx)
}
