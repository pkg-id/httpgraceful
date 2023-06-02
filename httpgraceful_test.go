package httpgraceful_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pkg-id/httpgraceful"
)

func ExampleWrapServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "Hello, World")
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", 8080),
		Handler: mux,
	}

	gs := httpgraceful.WrapServer(server)
	if err := gs.ListenAndServe(); err != nil {
		panic(err)
	}
}

func TestServer_ListenAndServe_CouldNotListen(t *testing.T) {
	var dummyErr = errors.New("dummy error")
	server := &serverMock{
		tracer:             visitedNone,
		ListenAndServeFunc: listener(0, dummyErr),
		ShutdownFunc: func(ctx context.Context) error {
			t.Error("should not be called")
			return nil
		},
		CloseFunc: func() error {
			t.Error("should not be called")
			return nil
		},
	}

	gs := httpgraceful.WrapServer(server)
	err := gs.ListenAndServe()
	expectTrue(t, errors.Is(err, dummyErr))
	tracer := server.Tracer()
	expectTrue(t, tracer.has(listenAndServeVisited))
	expectTrue(t, tracer.absent(shutdownVisited))
	expectTrue(t, tracer.absent(closeVisited))
}

func TestServer_ListenAndServe_ShutdownGracefully(t *testing.T) {
	server := &serverMock{
		tracer:             visitedNone,
		ListenAndServeFunc: listener(100*time.Millisecond, http.ErrServerClosed),
		ShutdownFunc:       shutdown(nil),
		CloseFunc: func() error {
			t.Error("should not be called")
			return nil
		},
	}

	gs := httpgraceful.WrapServer(server)
	triggerShutdownAfter(t, gs, os.Interrupt, 50*time.Millisecond)
	err := gs.ListenAndServe()
	expectNoErr(t, err)
	tracer := server.Tracer()
	expectTrue(t, tracer.has(listenAndServeVisited))
	expectTrue(t, tracer.has(shutdownVisited))
	expectTrue(t, tracer.absent(closeVisited))
}

func TestServer_ListenAndServe_ShutdownGracefully_ButFailed(t *testing.T) {
	dummyErr := errors.New("dummy error")
	server := &serverMock{
		tracer:             visitedNone,
		ListenAndServeFunc: listener(100*time.Millisecond, http.ErrServerClosed),
		ShutdownFunc:       shutdown(dummyErr),
		CloseFunc: func() error {
			t.Error("should not be called")
			return nil
		},
	}

	gs := httpgraceful.WrapServer(server)
	triggerShutdownAfter(t, gs, os.Interrupt, 50*time.Millisecond)
	err := gs.ListenAndServe()
	expectTrue(t, errors.Is(err, dummyErr))
	tracer := server.Tracer()
	expectTrue(t, tracer.has(listenAndServeVisited))
	expectTrue(t, tracer.has(shutdownVisited))
	expectTrue(t, tracer.absent(closeVisited))
}

func TestServer_ListenAndServe_ShutdownForcefully(t *testing.T) {
	server := &serverMock{
		tracer:             visitedNone,
		ListenAndServeFunc: listener(100*time.Millisecond, http.ErrServerClosed),
		ShutdownFunc:       shutdown(context.DeadlineExceeded),
		CloseFunc:          func() error { return nil },
	}

	gs := httpgraceful.WrapServer(server, httpgraceful.WithWaitTimeout(100*time.Millisecond))
	triggerShutdownAfter(t, gs, os.Interrupt, 50*time.Millisecond)

	err := gs.ListenAndServe()
	expectNoErr(t, err)
	tracer := server.Tracer()
	expectTrue(t, tracer.has(listenAndServeVisited))
	expectTrue(t, tracer.has(shutdownVisited))
	expectTrue(t, tracer.has(closeVisited))
}

func TestServer_ListenAndServe_ShutdownForcefully_ButFailed(t *testing.T) {
	dummyErr := errors.New("dummy error")
	server := &serverMock{
		tracer:             visitedNone,
		ListenAndServeFunc: listener(100*time.Millisecond, http.ErrServerClosed),
		ShutdownFunc:       shutdown(context.DeadlineExceeded),
		CloseFunc:          func() error { return dummyErr },
	}

	gs := httpgraceful.WrapServer(server, httpgraceful.WithWaitTimeout(100*time.Millisecond))
	triggerShutdownAfter(t, gs, os.Interrupt, 50*time.Millisecond)

	err := gs.ListenAndServe()
	expectTrue(t, errors.Is(err, dummyErr))
	tracer := server.Tracer()
	expectTrue(t, tracer.has(listenAndServeVisited))
	expectTrue(t, tracer.has(shutdownVisited))
	expectTrue(t, tracer.has(closeVisited))
}

func triggerShutdownAfter(t *testing.T, server httpgraceful.Server, sig os.Signal, timeout time.Duration) {
	time.AfterFunc(timeout, func() {
		sender, ok := server.(interface{ SendSignal(os.Signal) })
		expectTrue(t, ok)
		sender.SendSignal(sig)
	})
}

func expectTrue(t *testing.T, exp bool) {
	t.Helper()
	if !exp {
		t.Errorf("expect true")
	}
}

func expectNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Error("expect error nil")
	}
}

type serverMock struct {
	ListenAndServeFunc func() error
	ShutdownFunc       func(ctx context.Context) error
	CloseFunc          func() error
	tracer             visitedFlags
	sync.RWMutex
}

func (s *serverMock) ListenAndServe() error {
	s.Lock()
	s.tracer = s.tracer.visit(listenAndServeVisited)
	s.Unlock()
	return s.ListenAndServeFunc()
}
func (s *serverMock) Shutdown(ctx context.Context) error {
	s.Lock()
	s.tracer = s.tracer.visit(shutdownVisited)
	s.Unlock()
	return s.ShutdownFunc(ctx)
}
func (s *serverMock) Close() error {
	s.Lock()
	s.tracer = s.tracer.visit(closeVisited)
	s.Unlock()
	return s.CloseFunc()
}

func (s *serverMock) Tracer() visitedFlags {
	s.RLock()
	defer s.RUnlock()
	return s.tracer
}

type visitedFlags int

func (f visitedFlags) has(flag visitedFlags) bool           { return f&flag != 0 }
func (f visitedFlags) absent(flag visitedFlags) bool        { return !f.has(flag) }
func (f visitedFlags) visit(flag visitedFlags) visitedFlags { return f | flag }

const (
	visitedNone visitedFlags = 1 << iota
	listenAndServeVisited
	shutdownVisited
	closeVisited
)

func listener(sleep time.Duration, err error) func() error {
	return func() error {
		<-time.After(sleep)
		return err
	}
}

func shutdown(err error) func(context.Context) error {
	return func(ctx context.Context) error {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			return errors.New("context should have deadline")
		}

		return err
	}
}
