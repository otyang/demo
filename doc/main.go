// Command doc serves the API reference: openapi.yaml, rendered with Scalar.
// It is self-contained: the spec is embedded from this folder and it uses
// only the standard library.
//
//	go run ./doc                      # serves on addr below
//	go run ./doc -addr 127.0.0.1:9000 # or anywhere else
//
// openapi.yaml describes every route the API serves; internal/api's tests
// fail when the two disagree.
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// addr is the host and port the docs are served on; the -addr flag
// overrides it.
var addr = "localhost:8090"

// specPath is where the spec is served; the page loads it from here.
const specPath = "/docs/openapig.yaml"

//go:embed openapi.yaml
var spec string

// referencePage renders the spec with Scalar. The script is pinned to a
// version; update it on purpose.
const referencePage = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Demo API reference</title>
</head>
<body>
  <div id="app"></div>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.73.1"></script>
  <script>
    Scalar.createApiReference("#app", { darkMode: true, url: "` + specPath + `" });
  </script>
</body>
</html>
`

func main() {
	flag.StringVar(&addr, "addr", addr, "host:port to serve the docs on")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listen", "address", addr, "error", err)
		os.Exit(1)
	}
	if err := serve(ctx, listener, handler(), logger); err != nil {
		logger.Error("docs server stopped", "error", err)
		os.Exit(1)
	}
}

// handler serves the page at /docs and the spec at specPath.
func handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /doc", serveText("text/html; charset=utf-8", referencePage))
	mux.HandleFunc("GET "+specPath, serveText("application/yaml; charset=utf-8", spec))
	return mux
}

func serveText(contentType, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		fmt.Fprint(w, body)
	}
}

// serve runs the server on listener until ctx is cancelled, then shuts it
// down, giving requests in flight up to 10 seconds to finish.
func serve(ctx context.Context, listener net.Listener, handler http.Handler, logger *slog.Logger) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("docs listening", "url", "http://"+listener.Addr().String()+"/docs")
		if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}

	logger.Info("docs shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return err
	}
	return <-serverErr
}
