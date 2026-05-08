package httpx_test

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/danilloboing/marketplace-golang/internal/core/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_StartAndShutdown(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/x", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})

	srv := httpx.NewServer(httpx.ServerOptions{
		Addr:            "127.0.0.1:0",
		Handler:         mux,
		ShutdownTimeout: 2 * time.Second,
	})

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	require.Eventually(t, func() bool { return srv.Addr() != "" }, 2*time.Second, 10*time.Millisecond)

	resp, err := http.Get("http://" + srv.Addr() + "/x")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	require.NoError(t, srv.Shutdown(context.Background()))

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop within deadline")
	}
}
