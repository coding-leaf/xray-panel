package app_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"panel/internal/app"
)

func TestServiceFunc(t *testing.T) {
	called := false
	expectedErr := errors.New("boom")

	svc := app.ServiceFunc(func(ctx context.Context) error {
		called = true
		return expectedErr
	})

	ctx := context.Background()
	err := svc.Start(ctx)
	if !called {
		t.Fatal("expected service func to be called")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestServiceGracefulExit(t *testing.T) {
	svc := app.ServiceFunc(func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- svc.Start(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error on graceful exit, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("service did not exit in a timely manner")
	}
}

// TestServiceOrchestration_CascadeError tests that when one service fails,
// errgroup cancels the group context and cascades to all other services to shut down.
func TestServiceOrchestration_CascadeError(t *testing.T) {
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, gCtx := errgroup.WithContext(rootCtx)

	errServiceFail := errors.New("fatal service failure")

	var s1Exited, s2Exited atomic.Bool

	// S1: Normal long-running service
	s1 := app.ServiceFunc(func(ctx context.Context) error {
		<-ctx.Done()
		s1Exited.Store(true)
		return nil
	})

	// S2: Failing service
	s2 := app.ServiceFunc(func(ctx context.Context) error {
		time.Sleep(30 * time.Millisecond)
		s2Exited.Store(true)
		return errServiceFail
	})

	// S3: Another normal service
	var s3Exited atomic.Bool
	s3 := app.ServiceFunc(func(ctx context.Context) error {
		<-ctx.Done()
		s3Exited.Store(true)
		return nil
	})

	services := []app.Service{s1, s2, s3}
	for _, s := range services {
		svc := s
		g.Go(func() error {
			return svc.Start(gCtx)
		})
	}

	err := g.Wait()
	if !errors.Is(err, errServiceFail) {
		t.Fatalf("expected errgroup to return %v, got %v", errServiceFail, err)
	}

	if !s1Exited.Load() {
		t.Error("expected s1 to be cascaded and shut down")
	}
	if !s2Exited.Load() {
		t.Error("expected s2 to have executed")
	}
	if !s3Exited.Load() {
		t.Error("expected s3 to be cascaded and shut down")
	}
}

// TestServiceOrchestration_CleanShutdown tests that when rootCtx is canceled,
// all services exit cleanly and g.Wait() returns nil.
func TestServiceOrchestration_CleanShutdown(t *testing.T) {
	rootCtx, cancel := context.WithCancel(context.Background())

	g, gCtx := errgroup.WithContext(rootCtx)

	var s1Clean, s2Clean atomic.Bool

	s1 := app.ServiceFunc(func(ctx context.Context) error {
		<-ctx.Done()
		s1Clean.Store(true)
		return nil
	})

	s2 := app.ServiceFunc(func(ctx context.Context) error {
		<-ctx.Done()
		s2Clean.Store(true)
		return nil
	})

	for _, s := range []app.Service{s1, s2} {
		svc := s
		g.Go(func() error {
			return svc.Start(gCtx)
		})
	}

	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := g.Wait(); err != nil {
		t.Fatalf("expected nil error on clean shutdown, got %v", err)
	}

	if !s1Clean.Load() || !s2Clean.Load() {
		t.Errorf("expected both services to exit cleanly, s1=%v, s2=%v", s1Clean.Load(), s2Clean.Load())
	}
}

// TestServiceOrchestration_PrematureExitCascades tests that if one service
// exits prematurely before rootCtx is canceled, the orchestrator
// recognizes this unexpected stop as an error and cascades cancellation to all services.
func TestServiceOrchestration_PrematureExitCascades(t *testing.T) {
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, gCtx := errgroup.WithContext(rootCtx)

	var s1Exited, s2Exited atomic.Bool

	// S1: Long-running service
	s1 := app.ServiceFunc(func(ctx context.Context) error {
		<-ctx.Done()
		s1Exited.Store(true)
		return nil
	})

	// S2: Prematurely stops with nil error
	s2 := app.ServiceFunc(func(ctx context.Context) error {
		time.Sleep(30 * time.Millisecond)
		s2Exited.Store(true)
		return nil // exits without error
	})

	services := []struct {
		name string
		svc  app.Service
	}{
		{"s1", s1},
		{"s2", s2},
	}

	for _, s := range services {
		entry := s
		g.Go(func() error {
			if err := entry.svc.Start(gCtx); err != nil {
				return err
			}
			if gCtx.Err() == nil {
				return errors.New("service stopped unexpectedly")
			}
			return nil
		})
	}

	err := g.Wait()
	if err == nil {
		t.Fatal("expected non-nil error when a service prematurely stops")
	}

	if !s1Exited.Load() {
		t.Error("expected s1 to be cascaded and shut down")
	}
	if !s2Exited.Load() {
		t.Error("expected s2 to have executed")
	}
}

