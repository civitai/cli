package civitai

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// civitai/cli#627: snippet bounded the body with a BYTE index, so a body whose
// 500th byte fell inside a multi-byte rune was cut mid-rune and the SDK
// returned invalid UTF-8. The fixtures are the issue's: "模型文件" (3-byte
// runes) behind a 0-3 byte prefix walks the cut across every alignment, plus a
// 4-byte emoji run.
func TestSnippetCutsOnARuneBoundary(t *testing.T) {
	bodies := map[string]string{}
	for _, prefix := range []string{"", "v", "v2", "v2x"} {
		bodies["cjk prefix="+prefix] = prefix + strings.Repeat("模型文件", 200)
	}
	bodies["emoji prefix=v"] = "v" + strings.Repeat("😀", 200)

	for name, body := range bodies {
		got := snippet([]byte(body))
		if !utf8.ValidString(got) {
			t.Errorf("%s: snippet returned invalid UTF-8 (tail % x)", name, got[len(got)-6:])
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("%s: an over-long body must be marked truncated, got tail %q", name, got[len(got)-6:])
		}
		kept := strings.TrimSuffix(got, "…")
		if len(kept) > 500 {
			t.Errorf("%s: kept %d bytes, over the 500-byte budget", name, len(kept))
		}
		// Never cut more than one rune short of the budget (a 4-byte rune is the
		// widest), so the bound still means what it says.
		if len(kept) < 500-3 {
			t.Errorf("%s: kept only %d bytes; the cut should land within one rune of 500", name, len(kept))
		}
		if !strings.HasPrefix(body, kept) {
			t.Errorf("%s: the kept text is not a prefix of the body", name)
		}
	}
}

// TestSnippetLeavesAShortBodyAlone is an invariant guard (green before and
// after): a body within the budget is returned whole, with no ellipsis.
func TestSnippetLeavesAShortBodyAlone(t *testing.T) {
	body := strings.Repeat("模", 166) // 498 bytes
	if got := snippet([]byte(body)); got != body {
		t.Errorf("a %d-byte body was changed: %q", len(body), got)
	}
}
