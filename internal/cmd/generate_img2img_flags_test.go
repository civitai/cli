package cmd

import (
	"strings"
	"testing"
)

// The img2img cases in generate_img2img_test.go drive runGenerate directly, so
// none of them proves the flags are WIRED. This one inspects the real command:
// without it, --image could be unregistered (or registered as a non-repeatable
// StringVar) and every case there would still pass.
func TestGenerateImage_FlagsAreRegistered(t *testing.T) {
	cmd := newGenerateCmd()

	img := cmd.Flags().Lookup("image")
	if img == nil {
		t.Fatal("--image is not registered on `civitai generate`")
	}
	if img.Value.Type() != "stringArray" {
		t.Errorf("--image type = %q, want stringArray — it must be repeatable", img.Value.Type())
	}
	if err := cmd.Flags().Set("image", "a.png"); err != nil {
		t.Fatalf("set --image: %v", err)
	}
	if err := cmd.Flags().Set("image", "b.png"); err != nil {
		t.Fatalf("set --image a second time: %v", err)
	}
	if got := img.Value.String(); got != "[a.png,b.png]" {
		t.Errorf("--image accumulated %q, want both values", got)
	}

	eco := cmd.Flags().Lookup("ecosystem")
	if eco == nil {
		t.Fatal("--ecosystem is not registered on `civitai generate`")
	}
	if eco.Value.Type() != "string" {
		t.Errorf("--ecosystem type = %q, want string", eco.Value.Type())
	}

	// pflag's UnquoteUsage treats the first back-quoted span as the flag's VALUE
	// NAME, which silently mangles --help. The repo already pins this for the
	// existing flags; the two new ones must follow.
	for _, f := range []string{"image", "ecosystem"} {
		if strings.Contains(cmd.Flags().Lookup(f).Usage, "`") {
			t.Errorf("--%s usage contains a back-quote, which pflag reads as the value name", f)
		}
	}
}
