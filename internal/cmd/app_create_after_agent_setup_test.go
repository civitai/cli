package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/civitai/cli/internal/manifest"
	"github.com/civitai/cli/internal/scaffold"
)

// The onboarding order is `civitai agent-setup` in an empty directory, then
// `civitai app create` — and the second step used to refuse the directory the
// first one had just written into:
//
//	Error: . is not empty — refusing to overwrite. Scaffold somewhere else …
//
// Measured on a blind dogfood run, which ended with the app in a subdirectory
// and two AGENTS.md files. These tests run the two REAL commands in that order
// rather than seeding files by hand, so the fixture cannot drift from what
// `agent-setup` actually writes.

// relFilesUnder is filesUnder keyed by slash-separated path relative to root.
func relFilesUnder(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for p, body := range filesUnder(t, root) {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			t.Fatalf("rel %s: %v", p, err)
		}
		out[filepath.ToSlash(rel)] = body
	}
	return out
}

// keptPathSet is agentSetupProjectPaths as slash-joined strings.
func keptPathSet() map[string]bool {
	set := map[string]bool{}
	for _, p := range agentSetupProjectPaths() {
		set[strings.Join(p, "/")] = true
	}
	return set
}

// TestAppCreateAcceptsADirectoryHoldingOnlyAgentSetupsFiles is the regression
// test, once per agent `agent-setup` writes a config for. The agent list is
// derived from the table, so a new project-scoped agent is covered on arrival.
func TestAppCreateAcceptsADirectoryHoldingOnlyAgentSetupsFiles(t *testing.T) {
	for _, agent := range agentsWithConfigFiles() {
		t.Run(agent, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agent); err != nil {
				t.Fatalf("agent-setup --agent %s: %v", agent, err)
			}
			before := relFilesUnder(t, dir)

			// Positive control: setup must have written into the project, or the
			// create below is scaffolding into an empty directory and proves
			// nothing about the case under test.
			if _, ok := before[agentsFilename]; !ok || len(before) < 2 {
				t.Fatalf("PREMISE BROKEN: agent-setup left %d file(s) in the project, want AGENTS.md and at least "+
					"one more: %v", len(before), keys(before))
			}
			// 🔴 THE SEAM. Everything setup wrote into the project has to be in
			// the set the scaffolder tolerates; a file setup starts writing that
			// agentSetupProjectPaths does not name brings the refusal straight
			// back, for that file's users only.
			kept := keptPathSet()
			for rel := range before {
				if !kept[rel] {
					t.Fatalf("agent-setup wrote %s into the project, which agentSetupProjectPaths does not name — "+
						"`app create` will refuse this directory", rel)
				}
			}

			out, errOut, err := run(t, "app", "create", "demo-app", "--dir", dir, "--yes")
			if err != nil {
				t.Fatalf("app create refused a directory holding only agent-setup's own files %v: %v\n%s\n%s",
					keys(before), err, out, errOut)
			}
			if _, statErr := os.Stat(filepath.Join(dir, manifest.Filename)); statErr != nil {
				t.Fatalf("app create reported success but wrote no %s: %v", manifest.Filename, statErr)
			}

			after := relFilesUnder(t, dir)
			for rel, was := range before {
				now, ok := after[rel]
				if !ok {
					t.Errorf("%s was removed by the scaffold", rel)
					continue
				}
				// AGENTS.md is the one file the scaffolder is MEANT to touch: it
				// re-derives the managed block for the project that now exists
				// (writeScaffoldAgentsMD). Asserted on its own below.
				if rel == agentsFilename {
					continue
				}
				if now != was {
					t.Errorf("%s changed across the scaffold; agent-setup's files must be left exactly as they were\n"+
						"before:\n%s\nafter:\n%s", rel, was, now)
				}
			}
			agents := after[agentsFilename]
			if agentsMDBlockState(agents) != blockPresent {
				t.Errorf("AGENTS.md no longer carries exactly one managed block after the scaffold:\n%s", agents)
			}
			if strings.Contains(agents, "No Civitai App has been scaffolded in this directory") {
				t.Errorf("AGENTS.md still describes an empty directory after `app create` put a project in it")
			}
		})
	}
}

// TestAppCreateLeavesTheAuthorsTextAroundTheManagedBlockAlone covers the half of
// "left as they were" the test above skips for AGENTS.md: the scaffolder rewrites
// the span between the markers, and an author's own text on either side of it
// survives byte for byte.
func TestAppCreateLeavesTheAuthorsTextAroundTheManagedBlockAlone(t *testing.T) {
	dir, _ := agentSetupProject(t)
	if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", agentClaude); err != nil {
		t.Fatalf("agent-setup: %v", err)
	}
	path := filepath.Join(dir, agentsFilename)
	block, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const head, tail = "# My project\n\nUse tabs, not spaces.\n\n", "\n## Deploys\n\nAsk Priya first.\n"
	// The file setup wrote is the block plus one trailing newline; drop it so the
	// END marker is immediately followed by `tail` and the suffix check is exact.
	if err := os.WriteFile(path, []byte(head+strings.TrimRight(string(block), "\n")+tail), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, errOut, err := run(t, "app", "create", "demo-app", "--dir", dir, "--yes"); err != nil {
		t.Fatalf("app create: %v\n%s\n%s", err, out, errOut)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), head+agentsBeginMarker) {
		t.Errorf("the author's text ABOVE the managed block changed:\n%s", got)
	}
	if !strings.HasSuffix(string(got), agentsEndMarker+tail) {
		t.Errorf("the author's text BELOW the managed block changed:\n%s", got)
	}
}

// TestAppCreateStillRefusesAnythingThatIsNotAgentSetups is the other side: the
// tolerance is for agent-setup's files and nothing else. Each case starts from a
// REAL setup run and adds or swaps exactly one thing, and each must get the
// refusal it always got, with the directory left exactly as it was.
func TestAppCreateStillRefusesAnythingThatIsNotAgentSetups(t *testing.T) {
	cases := []struct {
		name  string
		agent string
		// mutate changes the directory setup produced. It returns false when
		// the platform cannot build the fixture (a symlink on Windows).
		mutate func(t *testing.T, dir string) bool
	}{
		{
			name: "one foreign file beside the setup files", agent: agentClaude,
			mutate: func(t *testing.T, dir string) bool {
				writeFixtureFile(t, filepath.Join(dir, "notes.txt"), "mine\n")
				return true
			},
		},
		{
			name: "a DIRECTORY named like a setup file", agent: agentClaude,
			mutate: func(t *testing.T, dir string) bool {
				removeFixtureFile(t, filepath.Join(dir, claudeFilename))
				if err := os.Mkdir(filepath.Join(dir, claudeFilename), 0o755); err != nil {
					t.Fatal(err)
				}
				return true
			},
		},
		{
			name: "a SYMLINK named like a setup file", agent: agentClaude,
			mutate: func(t *testing.T, dir string) bool {
				// The link points at a real regular file with the same content,
				// so only the entry's own type can tell it from the original.
				target := filepath.Join(t.TempDir(), "elsewhere.md")
				writeFixtureFile(t, target, claudeShim)
				removeFixtureFile(t, filepath.Join(dir, claudeFilename))
				if err := os.Symlink(target, filepath.Join(dir, claudeFilename)); err != nil {
					t.Skipf("cannot create a symlink here: %v", err)
					return false
				}
				return true
			},
		},
		{
			name: "a foreign file inside a setup DIRECTORY", agent: agentVSCode,
			mutate: func(t *testing.T, dir string) bool {
				writeFixtureFile(t, filepath.Join(dir, ".vscode", "settings.json"), "{}\n")
				return true
			},
		},
		{
			name: "a SYMLINK where setup writes a directory", agent: agentVSCode,
			mutate: func(t *testing.T, dir string) bool {
				// Move setup's real `.vscode/` out of the project and link to it:
				// the contents are exactly what setup wrote, the entry is a link.
				moved := filepath.Join(t.TempDir(), "vscode-elsewhere")
				if err := os.Rename(filepath.Join(dir, ".vscode"), moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, filepath.Join(dir, ".vscode")); err != nil {
					t.Skipf("cannot create a symlink here: %v", err)
					return false
				}
				return true
			},
		},
		{
			name: "a regular FILE where setup writes a directory", agent: agentClaude,
			mutate: func(t *testing.T, dir string) bool {
				writeFixtureFile(t, filepath.Join(dir, ".vscode"), "not a directory\n")
				return true
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := agentSetupProject(t)
			if _, _, err := run(t, "agent-setup", "--dir", dir, "--agent", tc.agent); err != nil {
				t.Fatalf("agent-setup: %v", err)
			}
			if !tc.mutate(t, dir) {
				return
			}
			before := dirListing(t, dir)

			_, _, err := run(t, "app", "create", "demo-app", "--dir", dir, "--yes")
			if err == nil {
				t.Fatalf("app create scaffolded into a directory holding something that is not agent-setup's: %v", before)
			}
			// The refusal is the existing one, word for word.
			if want := dir + " is not empty — " + scaffold.NotEmptyRemedy; err.Error() != want {
				t.Errorf("refusal changed\n got: %s\nwant: %s", err.Error(), want)
			}
			if after := dirListing(t, dir); strings.Join(after, "\n") != strings.Join(before, "\n") {
				t.Errorf("a refused scaffold modified the directory\nbefore: %v\n after: %v", before, after)
			}
		})
	}
}

// TestAppCreateIntoATrulyEmptyDirectoryStillWorks is the control for the two
// tests above: an existing empty directory, which never refused.
//
// ⚠ INVARIANT GUARD, not regression coverage — it is green before this change.
func TestAppCreateIntoATrulyEmptyDirectoryStillWorks(t *testing.T) {
	dir, _ := agentSetupProject(t)
	if out, errOut, err := run(t, "app", "create", "demo-app", "--dir", dir, "--yes"); err != nil {
		t.Fatalf("app create into an empty directory: %v\n%s\n%s", err, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, manifest.Filename)); err != nil {
		t.Fatalf("no %s after a successful scaffold: %v", manifest.Filename, err)
	}
}

// TestNoTemplateEmitsAKeptPath pins the fact scaffold.RenderKeeping's promise
// rests on today: no template writes a path agent-setup also writes, so
// tolerating those files can never mean writing over one. RenderKeeping refuses
// at run time if this ever stops holding; this is what says so at build time,
// naming the template and the path.
func TestNoTemplateEmitsAKeptPath(t *testing.T) {
	kept := agentSetupProjectPaths()
	if len(kept) < 3 {
		t.Fatalf("agentSetupProjectPaths returned %d path(s) — the comparison below would be near-vacuous: %v", len(kept), kept)
	}
	for _, tmpl := range scaffold.AllTemplates() {
		outs, err := scaffold.OutputPaths(tmpl)
		if err != nil {
			t.Fatalf("%s: %v", tmpl, err)
		}
		if len(outs) == 0 {
			t.Fatalf("%s: no output paths — the walk is reading the wrong tree", tmpl)
		}
		for _, out := range outs {
			out = filepath.ToSlash(out)
			for _, k := range kept {
				// A collision is the same path, or either being a parent of the
				// other (a template file INSIDE `.vscode/` also lands on it).
				kp := strings.Join(k, "/")
				if out == kp || strings.HasPrefix(out, kp+"/") || strings.HasPrefix(kp, out+"/") ||
					strings.HasPrefix(out, k[0]+"/") {
					t.Errorf("template %s writes %s, which collides with agent-setup's %s", tmpl, out, kp)
				}
			}
		}
	}
}

// TestAgentSetupProjectPathsIsTheProjectScopedSet pins the derivation against
// literal names, because every other test here reads the function it is testing.
func TestAgentSetupProjectPathsIsTheProjectScopedSet(t *testing.T) {
	var got []string
	for p := range keptPathSet() {
		got = append(got, p)
	}
	sort.Strings(got)
	want := []string{".cursor/mcp.json", ".mcp.json", ".vscode/mcp.json", "AGENTS.md", "CLAUDE.md", "opencode.json"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("agentSetupProjectPaths\n got: %v\nwant: %v\n"+
			"If an agent was added or moved between project and user scope, update this list.", got, want)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// dirListing is every entry under root with its type, so a comparison notices a
// swapped kind as well as an added or removed name.
func dirListing(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, info.Mode().Type().String()+" "+filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(out)
	return out
}

func writeFixtureFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func removeFixtureFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
