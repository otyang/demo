package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServeShutsDownOnContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	server := &http.Server{Handler: handler}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		result <- serve(ctx, "test", server, listener, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusNoContent, response.StatusCode)

	cancel()
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down after context cancellation")
	}
}

func TestBootstrapRejectsNilHandler(t *testing.T) {
	err := Bootstrap(context.Background(), "test", "127.0.0.1:0", nil, nil)
	require.ErrorContains(t, err, "handler cannot be nil")
}

func TestBootstrapRejectsNilContext(t *testing.T) {
	var ctx context.Context // nil: Bootstrap must reject it, not start serving
	err := Bootstrap(ctx, "test", "127.0.0.1:0", http.NotFoundHandler(), nil)
	require.ErrorContains(t, err, "context cannot be nil")
}

// TestShutdownLetsInFlightRequestsFinish cancels the server context while a
// request is running. The request's own context must stay alive, and the
// request must complete with its normal response.
func TestShutdownLetsInFlightRequestsFinish(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	started := make(chan struct{})
	release := make(chan struct{})
	requestCtxErr := make(chan error, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		requestCtxErr <- r.Context().Err()
		w.WriteHeader(http.StatusNoContent)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := newServer(ctx, listener.Addr().String(), handler)
	result := make(chan error, 1)
	go func() {
		result <- serve(ctx, "test", server, listener, slog.New(slog.DiscardHandler))
	}()

	responses := make(chan *http.Response, 1)
	go func() {
		response, err := (&http.Client{Timeout: 5 * time.Second}).Get("http://" + listener.Addr().String())
		if err == nil {
			responses <- response
		}
		close(responses)
	}()

	<-started
	cancel() // begin graceful shutdown while the request is in flight
	time.Sleep(50 * time.Millisecond)
	close(release)

	require.NoError(t, <-requestCtxErr, "request context must not be cancelled by shutdown")
	response, ok := <-responses
	require.True(t, ok, "in-flight request failed")
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not finish shutting down")
	}
}
