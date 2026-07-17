package cmd

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/civitai/cli/internal/api"
)

// TestDownloadOneCancelCleansPart proves a context cancellation mid-transfer
// (what SIGINT triggers via signal.NotifyContext) unblocks the streaming copy
// with an error and leaves NO .part (and no final file) behind — the cleanup
// defer fires on the cancelled io.Copy.
func TestDownloadOneCancelCleansPart(t *testing.T) {
	streaming := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Promise a large body, send a first chunk, flush, then block until the
		// client cancels (its ctx cancellation closes the connection -> r.Context()
		// is Done here), so the client's io.Copy is stuck mid-stream when we cancel.
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "partial-bytes")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(streaming)
		<-r.Context().Done()
	}))
	defer srv.Close()

	dir := t.TempDir()
	target := filepath.Join(dir, "m.safetensors")
	dl := api.New(srv.URL, "", "")
	f := api.ModelVersionFile{Name: "m.safetensors", Type: "Model", DownloadURL: srv.URL + "/dl"}
	o := &downloadOpts{noVerify: true}

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := downloadOne(ctx, dl, io.Discard, io.Discard, f, target, o)
		errc <- err
	}()

	<-streaming // the transfer has started and is now blocked
	cancel()    // simulate Ctrl-C

	err := <-errc
	if err == nil {
		t.Fatal("expected a cancellation error from the interrupted download")
	}
	if _, e := os.Stat(target + ".part"); !os.IsNotExist(e) {
		t.Errorf(".part should be removed on cancel; stat err = %v", e)
	}
	if _, e := os.Stat(target); !os.IsNotExist(e) {
		t.Errorf("no final file should exist on cancel; stat err = %v", e)
	}
}
