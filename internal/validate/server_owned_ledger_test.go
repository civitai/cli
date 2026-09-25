package validate

import (
	"strings"
	"testing"
)

// THE LEDGER: ServerOwnedFields and serverOwnedFieldChecks must name the same
// set, in both directions.
//
// 🔴 WHY A LEDGER AND NOT A COMMENT. The list is now read by TWO consumers —
// this package's validator, and `app submit`'s manifest-drift comparison, which
// must IGNORE exactly these fields. A field present in one and missing from the
// other fails silently and in opposite ways: a field in the checks but not the
// list makes the drift warning fire on every app forever; a field in the list
// but not the checks makes the validator accept something the platform will
// overwrite. Neither shows up as a test failure anywhere else.

// setField builds a manifest with one dotted-path field set, so each entry can
// be driven independently. Only the one- and two-segment shapes the list uses
// are supported — a deeper path is an error rather than a silent miss, because
// a path this helper cannot build would otherwise look like a passing case.
func setField(t *testing.T, path string) map[string]any {
	t.Helper()
	parts := strings.Split(path, ".")
	switch len(parts) {
	case 1:
		return map[string]any{parts[0]: "probe"}
	case 2:
		return map[string]any{parts[0]: map[string]any{parts[1]: "probe"}}
	default:
		t.Fatalf("setField cannot build %q (%d segments) — extend it rather than skipping the entry", path, len(parts))
		return nil
	}
}

// TestServerOwnedFieldChecksCoverTheList — every entry must be REJECTED, and
// the finding must name that field.
func TestServerOwnedFieldChecksCoverTheList(t *testing.T) {
	if len(ServerOwnedFields) == 0 {
		t.Fatal("ServerOwnedFields is empty — this guard would be asserting nothing")
	}
	for _, path := range ServerOwnedFields {
		findings := serverOwnedFieldChecks(setField(t, path))
		if len(findings) == 0 {
			t.Errorf("ServerOwnedFields names %q but serverOwnedFieldChecks does not reject it — "+
				"the validator would accept a field the platform overwrites", path)
			continue
		}
		var named bool
		for _, f := range findings {
			if f.Field == path {
				named = true
			}
		}
		if !named {
			var got []string
			for _, f := range findings {
				got = append(got, f.Field)
			}
			t.Errorf("setting %q produced findings for %v — none names %q, so the drift comparison "+
				"(which ignores by field path) and the validator disagree about what this field is called",
				path, got, path)
		}
	}
}

// TestServerOwnedFieldChecksRejectNothingElse is the other direction: a field
// the checks reject but the list omits would make `app submit`'s drift warning
// fire on every app, every time — the shape that trains people to ignore it.
//
// It works by feeding a manifest with EVERY listed field set and asserting the
// finding count matches the list length: an unlisted check would add a finding
// with nowhere to come from.
func TestServerOwnedFieldChecksRejectNothingElse(t *testing.T) {
	m := map[string]any{}
	for _, path := range ServerOwnedFields {
		for k, v := range setField(t, path) {
			if existing, ok := m[k].(map[string]any); ok {
				if add, ok := v.(map[string]any); ok {
					for ik, iv := range add {
						existing[ik] = iv
					}
					continue
				}
			}
			m[k] = v
		}
	}
	findings := serverOwnedFieldChecks(m)
	if len(findings) != len(ServerOwnedFields) {
		var got []string
		for _, f := range findings {
			got = append(got, f.Field)
		}
		t.Errorf("serverOwnedFieldChecks produced %d finding(s) %v for the %d listed field(s) %v — "+
			"a check with no ledger entry makes the drift warning fire forever; a missing one makes it blind",
			len(findings), got, len(ServerOwnedFields), ServerOwnedFields)
	}
}

// TestServerOwnedFieldsIsNotEmptyAndIsDotted guards the SHAPE the drift
// comparison depends on: it splits these on "." to walk the manifest, so an
// entry with a leading/trailing dot or whitespace would silently match nothing.
func TestServerOwnedFieldsIsNotEmptyAndIsDotted(t *testing.T) {
	for _, path := range ServerOwnedFields {
		if path == "" || strings.TrimSpace(path) != path {
			t.Errorf("ServerOwnedFields entry %q is not a clean field path", path)
		}
		for _, seg := range strings.Split(path, ".") {
			if seg == "" {
				t.Errorf("ServerOwnedFields entry %q has an empty segment — it would match nothing", path)
			}
		}
	}
}
