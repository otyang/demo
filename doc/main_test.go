package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestHandlerServesThePageAndTheSpec(t *testing.T) {
	for _, test := range []struct {
		method, path string
		status       int
		contentType  string
		body         string
	}{
		{http.MethodGet, "/docs", http.StatusOK, "text/html; charset=utf-8", `url: "/docs/openapi.yaml"`},
		{http.MethodGet, "/docs/openapi.yaml", http.StatusOK, "application/yaml; charset=utf-8", "openapi: 3.1.0"},
		{http.MethodPost, "/docs", http.StatusMethodNotAllowed, "", ""},
		{http.MethodGet, "/docs/other", http.StatusNotFound, "", ""},
	} {
		recorder := httptest.NewRecorder()
		handler().ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
		assert.Equal(t, test.status, recorder.Code, test.path)
		if test.contentType != "" {
			assert.Equal(t, test.contentType, recorder.Header().Get("Content-Type"), test.path)
			assert.Contains(t, recorder.Body.String(), test.body, test.path)
		}
	}
}

func TestSpecIsValidYAML(t *testing.T) {
	var document struct {
		OpenAPI string         `yaml:"openapi"`
		Paths   map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(spec), &document))
	assert.True(t, strings.HasPrefix(document.OpenAPI, "3."))
	assert.NotEmpty(t, document.Paths)
}

func TestServeShutsDownWhenTheContextIsCancelled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- serve(ctx, listener, handler(), slog.New(slog.DiscardHandler)) }()

	response, err := (&http.Client{Timeout: time.Second}).Get("http://" + listener.Addr().String() + "/docs/openapi.yaml")
	require.NoError(t, err)
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	assert.Equal(t, spec, string(body))

	cancel()
	select {
	case err := <-result:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
}
