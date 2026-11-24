package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRaw(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readRaw(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, Filename))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestSetRepositoryKeepsTheFileAsWritten pins the exact bytes for each spelling
// a manifest arrives in: the new pair follows blockId in its indent, line
// ending and colon spacing, and nothing else changes.
func TestSetRepositoryKeepsTheFileAsWritten(t *testing.T) {
	const url = "https://github.com/acme/my-app"
	cases := []struct{ name, in, want string }{
		{
			"two-space indent, blockId not last",
			"{\n  \"name\": \"X\",\n  \"blockId\": \"x\",\n  \"version\": \"1.0.0\"\n}\n",
			"{\n  \"name\": \"X\",\n  \"blockId\": \"x\",\n  \"repository\": \"https://github.com/acme/my-app\",\n  \"version\": \"1.0.0\"\n}\n",
		},
		{
			"tab indent, blockId last",
			"{\n\t\"version\": \"1.0.0\",\n\t\"blockId\": \"x\"\n}",
			"{\n\t\"version\": \"1.0.0\",\n\t\"blockId\": \"x\",\n\t\"repository\": \"https://github.com/acme/my-app\"\n}",
		},
		{
			"CRLF",
			"{\r\n    \"blockId\" : \"x\",\r\n    \"version\" : \"1.0.0\"\r\n}\r\n",
			"{\r\n    \"blockId\" : \"x\",\r\n    \"repository\" : \"https://github.com/acme/my-app\",\r\n    \"version\" : \"1.0.0\"\r\n}\r\n",
		},
		{
			"one line",
			`{"blockId":"x","version":"1.0.0"}`,
			`{"blockId":"x", "repository":"https://github.com/acme/my-app","version":"1.0.0"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeRaw(t, tc.in)
			if err := SetRepository(dir, url); err != nil {
				t.Fatalf("SetRepository: %v", err)
			}
			if got := readRaw(t, dir); got != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tc.want)
			}
			info, err := os.Stat(filepath.Join(dir, Filename))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o640 {
				t.Errorf("mode = %v, want 0640 kept", info.Mode().Perm())
			}
		})
	}
}

// TestSetRepositoryRefusesWithoutWriting: each refusal leaves the file
// byte-identical.
func TestSetRepositoryRefusesWithoutWriting(t *testing.T) {
	cases := []struct{ name, in, wantErr string }{
		{"already declared", "{\n  \"repository\": \"https://gitlab.com/o/r\",\n  \"blockId\": \"x\"\n}\n", "already declares"},
		{"declared null", "{\n  \"blockId\": \"x\",\n  \"repository\": null\n}\n", "already declares"},
		{"no blockId", "{\n  \"version\": \"1.0.0\"\n}\n", "no \"blockId\""},
		{"blockId only nested", "{\n  \"page\": { \"blockId\": \"x\" },\n  \"version\": \"1.0.0\"\n}\n", "top-level"},
		{"nested blockId first", "{\n  \"page\": {\n    \"blockId\": \"y\"\n  },\n  \"blockId\": \"x\"\n}\n", "top-level"},
		{"not JSON", "{\n  \"blockId\": \"x\",\n", "not valid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeRaw(t, tc.in)
			err := SetRepository(dir, "https://github.com/acme/my-app")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want a refusal containing %q, got %v", tc.wantErr, err)
			}
			if got := readRaw(t, dir); got != tc.in {
				t.Errorf("a refusal rewrote the file:\n%q", got)
			}
		})
	}
}

func TestHasKey(t *testing.T) {
	for in, want := range map[string]bool{
		`{"blockId":"x"}`:                     false,
		`{"blockId":"x","repository":null}`:   true,
		`{"blockId":"x","repository":""}`:     true,
		`{"page":{"repository":"https://x"}}`: false,
	} {
		got, err := HasKey(writeRaw(t, in), "repository")
		if err != nil || got != want {
			t.Errorf("HasKey(%s) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := HasKey(t.TempDir(), "repository"); err == nil {
		t.Error("a missing manifest must be an error, not false")
	}
}
