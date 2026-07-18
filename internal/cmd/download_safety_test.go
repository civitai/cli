package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPickleArchiveNote unit-tests the extension classifier: pickle/executable
// and archive extensions get the note; safetensors/images/other do not.
func TestPickleArchiveNote(t *testing.T) {
	withNote := []string{
		"model.ckpt", "weights.pt", "net.pth", "state.bin", "data.pickle", "x.pkl",
		"bundle.zip", "archive.tar", "archive.tar.gz", "archive.tgz", "pack.rar", "pack.7z",
		"MODEL.CKPT", "Archive.ZIP", // case-insensitive
		"/some/dir/nested.ckpt", // basename only
	}
	for _, name := range withNote {
		note := pickleArchiveNote(name)
		if note == "" {
			t.Errorf("%q should emit a pickle/archive note", name)
			continue
		}
		if !strings.Contains(note, "execute code") {
			t.Errorf("%q note should warn about code execution: %q", name, note)
		}
	}
	noNote := []string{
		"model.safetensors", "image.png", "photo.jpeg", "config.yaml", "readme.txt", "notes.json", "novo",
	}
	for _, name := range noNote {
		if note := pickleArchiveNote(name); note != "" {
			t.Errorf("%q should NOT emit a note, got %q", name, note)
		}
	}
}

// TestDownloadPickleArchiveNotePrinted proves the note is emitted to stderr for a
// .ckpt and a .zip download, and NOT for a .safetensors.
func TestDownloadPickleArchiveNotePrinted(t *testing.T) {
	cases := []struct {
		file     string
		wantNote bool
	}{
		{"model.ckpt", true},
		{"bundle.zip", true},
		{"model.safetensors", false},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			d := newDLServer(t, false, []dlFile{
				{id: 1, name: tc.file, typ: "Model", primary: true, body: "bytes", withSHA: true},
			})
			setupDownloadEnv(t, d, "")
			_, errOut, err := run(t, "download", "128713")
			if err != nil {
				t.Fatalf("download %s: %v", tc.file, err)
			}
			hasNote := strings.Contains(errOut, "pickle/archive-format file")
			if hasNote != tc.wantNote {
				t.Errorf("%s: note present = %v, want %v\nstderr: %s", tc.file, hasNote, tc.wantNote, errOut)
			}
			if tc.wantNote && !strings.Contains(errOut, tc.file) {
				t.Errorf("%s: note should name the file: %s", tc.file, errOut)
			}
		})
	}
}

// TestDownloadPlanSanitizesControlChars proves the --dry-run plan strips terminal
// control chars from the server-supplied file name (output forgery via the plan).
func TestDownloadPlanSanitizesControlChars(t *testing.T) {
	hostileName := forge("evil") + ".ckpt"
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/model-versions/", func(w http.ResponseWriter, r *http.Request) {
		// json.Marshal escapes the control chars in the name → valid JSON on the wire.
		fmt.Fprintf(w, `{"id":128713,"modelId":4384,"name":"v","baseModel":%s,"files":[{"id":1,"name":%s,"type":"Model","primary":true,"sizeKB":1,"downloadUrl":"%s/dl/x","hashes":{"SHA256":"abc"}}]}`,
			jsonString(forge("SD 1.5")), jsonString(hostileName), base)
	})
	srv := httptest.NewServer(mux)
	base = srv.URL
	t.Cleanup(srv.Close)

	allowPrivateDownloadHostsInTest(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CIVITAI_TOKEN", "")
	t.Setenv("CIVITAI_BASE_URL", srv.URL)
	chdir(t, t.TempDir())

	out, errOut, err := run(t, "download", "128713", "--dry-run")
	if err != nil {
		t.Fatalf("download --dry-run: %v", err)
	}
	assertNoControlBytes(t, "download plan stdout", out)
	assertNoControlBytes(t, "download plan stderr", errOut)
	if !strings.Contains(out, "evil") {
		t.Errorf("visible file-name text should survive the strip: %s", out)
	}
}
