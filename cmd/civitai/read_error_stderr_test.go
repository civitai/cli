package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/civitai/cli/pkg/civitai"
)

// TestReadErrorReachesStderrWithoutTerminalControlRunes is the SEAM guard for
// the civitai/cli#526 follow-up's (a) decision.
//
// 🔴 EACH SIDE OF THIS SEAM IS TESTED AND NEITHER SIDE OWNS IT.
// pkg/civitai proves snippet() strips; internal/cmd proves safeTerm strips on
// the HUMAN renderers. Nothing joined them, and the join is where the hole was:
// errorLine below hands err.Error() to fmt.Fprintln(os.Stderr, …) with no
// filter of its own, so before this a 404 body carrying ESC[1A ESC[2K could
// erase the CLI's own preceding output from inside an "Error: " line.
//
// This test builds the state neither package's fixtures build: a real HTTP
// reply, decoded by the real SDK, rendered by the real stderr renderer.
//
// It also pins the published EXIT CODE across the change (AGENTS.md item 7): a
// 404 must still classify as exit 4. The strip touches the message, and the
// message is not what carries the classification — this is the assertion that
// says so rather than assuming it.
func TestReadErrorReachesStderrWithoutTerminalControlRunes(t *testing.T) {
	// ESC[1A is CURSOR UP, ESC[2K is ERASE LINE, U+2066 is an isolate that can
	// reorder what follows. All three are written as escapes: a literal control
	// byte in a Go string literal is what staticcheck's ST1018 catches.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(
			"{\"message\":\"no version 44190\\u001b[1A\\u001b[2K\\u2066 checksum ok\"}"))
	}))
	t.Cleanup(srv.Close)

	_, _, err := civitai.New(srv.URL, "").GetModelVersion(context.Background(), "44190")
	if err == nil {
		t.Fatal("a 404 must be an error; without it this test asserts on nothing")
	}
	line := errorLine(err)

	// Positive control FIRST: the server's own words must be in the line, or the
	// absence checks below pass on an empty string.
	if !strings.Contains(line, "no version 44190") || !strings.Contains(line, "checksum ok") {
		t.Fatalf("the server's words are missing — the checks below would be vacuous:\n%q", line)
	}
	for _, bad := range []struct {
		r    rune
		name string
	}{
		{'\u001b', "ESC (0x1B), the ANSI/OSC introducer"},
		{'\u2066', "U+2066 LEFT-TO-RIGHT ISOLATE"},
	} {
		if strings.ContainsRune(line, bad.r) {
			t.Errorf("%s reached the line main() writes to stderr:\n%q", bad.name, line)
		}
	}
	// The published exit code for a 404 read is 4 and must not have moved.
	if got := exitCode(err); got != exitNotFound {
		t.Errorf("exit code = %d, want %d (exitNotFound) — a 404 read's published "+
			"classification must survive the message filter", got, exitNotFound)
	}
}
