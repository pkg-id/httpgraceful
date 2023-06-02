package httpgraceful

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Server is a contract for server that can be started and shutdown gracefully.
type Server interface {
	// ListenAndServe starts listening and serving the server.
	// This method should block until shutdown signal received or failed to start.
	ListenAndServe() error

	// Shutdown gracefully shuts down the server, it will wait for all active connections to be closed.
	Shutdown(ctx context.Context) error

	// Close force closes the server.
	// Close is called when Shutdown timeout exceeded.
	Close() error
}

// gracefulServer is a wrapper of http.Server that can be shutdown gracefully.
type gracefulServer struct {
	Server
	signalListener chan os.Signal
	waitTimeout    time.Duration
	shutdownDone   chan struct{}
}

// Option is a function to configure gracefulServer.
type Option func(*gracefulServer)

func (f Option) apply(gs *gracefulServer) { f(gs) }

// WithSignals sets the signals that will be listened to initiate shutdown.
func WithSignals(signals ...os.Signal) Option {
	return func(s *gracefulServer) {
		signalListener := make(chan os.Signal, 1)
		signal.Notify(signalListener, signals...)
		s.signalListener = signalListener
	}
}

// WithWaitTimeout sets the timeout for waiting active connections to be closed.
func WithWaitTimeout(timeout time.Duration) Option {
	return func(s *gracefulServer) {
		s.waitTimeout = timeout
	}
}

// WrapServer wraps a Server with graceful shutdown capability.
// It will listen to SIGINT and SIGTERM signals to initiate shutdown and
// wait for all active connections to be closed. If still active connections
// after wait timeout exceeded, it will force close the server. The default
// wait timeout is 5 seconds.
func WrapServer(server Server, opts ...Option) Server {
	gs := gracefulServer{
		Server:       server,
		shutdownDone: make(chan struct{}),
	}

	for _, opt := range opts {
		opt.apply(&gs)
	}

	if gs.signalListener == nil {
		WithSignals(syscall.SIGTERM, syscall.SIGINT).apply(&gs)
	}

	if gs.waitTimeout <= 0 {
		WithWaitTimeout(5 * time.Second).apply(&gs)
	}

	return &gs
}

// ListenAndServe starts listening and serving the server gracefully.
func (s *gracefulServer) ListenAndServe() error {
	serverErr := make(chan error, 1)
	shutdownCompleted := make(chan struct{})

	// start the original server.
	go func() {
		err := s.Server.ListenAndServe()
		// if shutdown succeeded, http.ErrServerClosed will be returned.
		if errors.Is(err, http.ErrServerClosed) {
			shutdownCompleted <- struct{}{}
		} else {
			// only send error if it's not http.ErrServerClosed.
			serverErr <- err
		}
	}()

	// block until signalListener received or mux failed to start.
	select {
	case sig := <-s.signalListener:
		ctx, cancel := context.WithTimeout(context.Background(), s.waitTimeout)
		defer cancel()

		err := s.Server.Shutdown(ctx)
		// only force shutdown if deadline exceeded.
		if errors.Is(err, context.DeadlineExceeded) {
			closeErr := s.Server.Close()
			if closeErr != nil {
				return fmt.Errorf("deadline exceeded, force shutdown failed: %w", closeErr)
			}
			// force shutdown succeeded.
			return nil
		}

		// unexpected error.
		if err != nil {
			return fmt.Errorf("shutdown failed, signal: %s: %w", sig, err)
		}

		// make sure shutdown completed.
		<-shutdownCompleted
		return nil
	case err := <-serverErr:
		return fmt.Errorf("server failed to start: %w", err)
	}
}

// SendSignal sends a signal to the server, if signal is one of registered signals,
// shutdown will be triggered.
// this useful for testing.
func (s *gracefulServer) SendSignal(sig os.Signal) {
	if s.signalListener != nil {
		s.signalListener <- sig
	}
}
