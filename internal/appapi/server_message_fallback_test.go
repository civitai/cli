package appapi

import (
	"net/http"
	"strings"
	"testing"
)

// TestServerMessageFallbackNeverEchoesTheBody pins serverMessage's fallback for a
// non-2xx body with no `message`/`error` field: it goes through
// redact.CredentialRouteMessage, so the error shows the body's size and never
// the body. It used to fall back to the trimmed raw body, so an edge 502 HTML
// page was printed whole — newlines and all, at whatever size it was.
//
// Every mapper that reads serverMessage is driven, because each one prints it.
func TestServerMessageFallbackNeverEchoesTheBody(t *testing.T) {
	html := "<html>\n<head><title>502 Bad Gateway</title></head>\n<body>\n" +
		"<center><h1>502 Bad Gateway</h1></center>\n<!-- " + bodyEchoSentinel + " -->\n" +
		"</body>\n</html>\n"
	huge := html + strings.Repeat("<p>"+bodyEchoSentinel+"</p>\n", 200*1024/32)

	mappers := map[string]func(int, []byte) error{
		"serverError":      serverError,
		"withdrawError":    withdrawError,
		"submissionsError": func(s int, raw []byte) error { return submissionsError(s, raw, "", "") },
		"devTunnelError":   devTunnelError,
	}
	for name, mapper := range mappers {
		for _, body := range []struct{ label, raw string }{{"html", html}, {"200KiB html", huge}} {
			t.Run(name+"/"+body.label, func(t *testing.T) {
				err := mapper(http.StatusBadGateway, []byte(body.raw))
				assertBodyNotEchoed(t, err, "502")
				if strings.Contains(err.Error(), "<") {
					t.Errorf("the error carries HTML markup from the body:\n  %.300q", err.Error())
				}
			})
		}
	}

	// The whole text, pinned for one mapper: the status and the body's size.
	if got, want := serverError(http.StatusBadGateway, []byte(html)).Error(),
		"server returned 502: 153-byte response body not shown because it may carry credentials"; got != want {
		t.Errorf("serverError(502, html) =\n  %q\nwant\n  %q", got, want)
	}

	// Positive control: a body WITH a message field still shows the server's
	// words, so the assertions above are not met by withholding everything.
	if got, want := serverError(http.StatusBadGateway, []byte(`{"message":"upstream deploy in progress"}`)).Error(),
		"server returned 502: upstream deploy in progress"; got != want {
		t.Errorf("control: serverError with a message field =\n  %q\nwant\n  %q", got, want)
	}
}
