package cmd

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture under testdata/elements-pkg is a TRIMMED copy of the published
// @civitai/components 0.10.0 (the version the page-elements template's `^0.10.0`
// floor resolves to): seven of its manifest modules with their public fields,
// and — verbatim — the compiled base classes the manifest does not describe
// (dist/elements/{base,field-base,media-base}.{js,d.ts}) plus the .d.ts files
// whose type aliases the manifest names. Its SHAPE is the point: the manifest's
// `<civitai-text-input>` lists five attributes and a `superclass` pointing at a
// module the manifest does not contain, exactly as the real file does.

// installElementsFixture copies the fixture package into
// <tmp>/proj/node_modules/@civitai/components and returns the package dir and
// a directory INSIDE the project (so the walk up is exercised, not skipped).
func installElementsFixture(t *testing.T) (pkgDir, workDir string) {
	t.Helper()
	proj := filepath.Join(t.TempDir(), "proj")
	pkgDir = filepath.Join(proj, "node_modules", "@civitai", "components")
	src := filepath.Join("testdata", "elements-pkg")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		dst := filepath.Join(pkgDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	workDir = filepath.Join(proj, "src", "views")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return pkgDir, workDir
}

func runElements(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("CIVITAI_NO_UPDATE_CHECK", "1")
	t.Setenv("NO_COLOR", "1")
	t.Chdir(dir)
	return run(t, append([]string{"app", "elements"}, args...)...)
}

// attrLine finds the rendered row for one attribute in a section.
func attrNames(fields []elementField) []string {
	var out []string
	for _, f := range fields {
		out = append(out, f.Name)
	}
	return out
}

func findField(fields []elementField, name string) (elementField, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return elementField{}, false
}

func TestAppElementsListsEveryTag(t *testing.T) {
	_, dir := installElementsFixture(t)
	out, _, err := runElements(t, dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []string{"civitai-button", "civitai-checkbox", "civitai-confirm-dialog", "civitai-image",
		"civitai-modal", "civitai-switch", "civitai-text-input"}
	if !strings.Contains(out, "@civitai/components 0.10.0 — 7 elements") {
		t.Errorf("list header should name the package, its version and the count:\n%s", out)
	}
	for _, tag := range want {
		if !strings.Contains(out, "  "+tag) {
			t.Errorf("list is missing %s:\n%s", tag, out)
		}
	}
	// The one-line description is the FIRST SENTENCE of a hard-wrapped one.
	if !strings.Contains(out, "civitai-switch          The checkbox wearing a track and a thumb.\n") {
		t.Errorf("list should carry each element's first sentence on its row:\n%s", out)
	}
	if !strings.Contains(out, "civitai app elements <tag>") {
		t.Errorf("list should name the per-element command:\n%s", out)
	}
}

// TestAppElementsFormControlIncludesInheritedAttributes is the reason the
// command exists: the manifest lists five attributes for <civitai-text-input>
// and none of the shared field attributes, which live on CivitaiField — a
// module the manifest does not contain.
func TestAppElementsFormControlIncludesInheritedAttributes(t *testing.T) {
	_, dir := installElementsFixture(t)
	out, _, err := runElements(t, dir, "civitai-text-input")
	if err != nil {
		t.Fatalf("text-input: %v", err)
	}
	if !strings.Contains(out, "<civitai-text-input>  CivitaiTextInput → CivitaiField → CivitaiElement") {
		t.Errorf("header should show the superclass chain:\n%s", out)
	}
	for _, own := range []string{"placeholder", "readonly", "maxlength", "autocomplete"} {
		if !strings.Contains(out, "\n  "+own+" ") {
			t.Errorf("own attribute %q missing:\n%s", own, out)
		}
	}
	// Inherited rows carry the declaring class, a type from the .d.ts (the
	// FieldSize alias expanded), and the constructor's default.
	for _, row := range []string{
		"  label         string              = ''  (CivitaiField)",
		"  required      boolean             = false  (CivitaiField)",
		"  size          'sm' | 'md' | 'lg'  = 'md'  (CivitaiField)",
		"  error         string              = ''  — Message to show; also marks the control invalid.  (CivitaiField)",
	} {
		if !strings.Contains(out, row+"\n") {
			t.Errorf("inherited row missing or malformed:\nwant: %q\n%s", row, out)
		}
	}
	for _, name := range []string{"name", "value", "description", "disabled"} {
		if !strings.Contains(out, "\n  "+name+" ") {
			t.Errorf("inherited attribute %q missing:\n%s", name, out)
		}
	}
	if strings.Contains(out, "Incomplete") {
		t.Errorf("a fully readable chain must not be reported incomplete:\n%s", out)
	}
	// A method's object-literal return type in the .d.ts is not a set of fields.
	for _, notField := range []string{"ariaInvalid", "ariaDescribedBy", "  part "} {
		if strings.Contains(out, notField) {
			t.Errorf("%q is a field of controlAttrs()'s RETURN TYPE, not of the element:\n%s", notField, out)
		}
	}
	if len(out) > 3000 {
		t.Errorf("one form control's reference is %d bytes; it should stay well under 3 KB", len(out))
	}
}

// TestAppElementsInheritanceIsResolvedNotListed proves the inherited rows come
// from the superclass walk: the same element resolved WITHOUT the walk has none
// of them. If this ever passes with followSuperclasses=false, the assertions in
// the test above are no longer evidence of inheritance resolution.
func TestAppElementsInheritanceIsResolvedNotListed(t *testing.T) {
	pkgDir, _ := installElementsFixture(t)
	pkg, err := loadElementsPackage(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	e, err := pkg.lookupElement("civitai-text-input")
	if err != nil {
		t.Fatal(err)
	}
	flat := pkg.resolveElement(e, false)
	if got := attrNames(flat.Attributes); strings.Join(got, ",") != "placeholder,type,readonly,maxlength,autocomplete" {
		t.Fatalf("without the walk the manifest's own five attributes are expected, got %v", got)
	}
	full := pkg.resolveElement(e, true)
	want := "placeholder,type,readonly,maxlength,autocomplete,name,value,label,description,error,required,disabled,size"
	if got := strings.Join(attrNames(full.Attributes), ","); got != want {
		t.Errorf("resolved attributes:\n got %s\nwant %s", got, want)
	}

	// A chain that passes THROUGH the manifest first: switch → checkbox (in
	// the manifest) → CivitaiField (not). checkbox overrides `value`, and the
	// nearer declaration must win.
	sw, _ := pkg.lookupElement("civitai-switch")
	ref := pkg.resolveElement(sw, true)
	if got := strings.Join(ref.Extends, ","); got != "CivitaiCheckbox,CivitaiField,CivitaiElement" {
		t.Errorf("switch chain = %s", got)
	}
	v, _ := findField(ref.Attributes, "value")
	if v.Default != "'on'" || v.DeclaredBy != "CivitaiCheckbox" {
		t.Errorf("switch `value` should be checkbox's override ('on'), got %+v", v)
	}
	// The whole list, so a shadowed member that ALSO appears (CivitaiField's
	// `value` after checkbox's) fails here rather than hiding behind a
	// first-match lookup.
	if got := strings.Join(attrNames(ref.Attributes), ","); got != "value,checked,indeterminate,name,label,description,error,required,disabled,size" {
		t.Errorf("switch attributes = %s", got)
	}
}

// TestAppElementsInheritanceWalkFollowsTheManifestGraph pins the manifest-only
// half of the walk on its own: strip the analyzer's pre-flattened inherited
// members from <civitai-confirm-dialog> and the CivitaiModal members must still
// arrive, via `superclass`, carrying the modal's class name.
func TestAppElementsInheritanceWalkFollowsTheManifestGraph(t *testing.T) {
	pkgDir, _ := installElementsFixture(t)
	pkg, err := loadElementsPackage(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	for mi := range pkg.Manifest.Modules {
		for di := range pkg.Manifest.Modules[mi].Declarations {
			d := &pkg.Manifest.Modules[mi].Declarations[di]
			if d.TagName != "civitai-confirm-dialog" {
				continue
			}
			var own []cemAttr
			for _, a := range d.Attributes {
				if a.InheritedFrom == nil {
					own = append(own, a)
				}
			}
			d.Attributes = own
			var ownEv []cemEvent
			for _, ev := range d.Events {
				if ev.InheritedFrom == nil {
					ownEv = append(ownEv, ev)
				}
			}
			d.Events = ownEv
		}
	}
	e, _ := pkg.lookupElement("civitai-confirm-dialog")
	if flat := pkg.resolveElement(e, false); len(flat.Attributes) != 4 {
		t.Fatalf("control: stripped confirm-dialog should carry only its 4 own attributes, got %v", attrNames(flat.Attributes))
	}
	ref := pkg.resolveElement(e, true)
	open, ok := findField(ref.Attributes, "open")
	if !ok || open.DeclaredBy != "CivitaiModal" {
		t.Errorf("`open` should be inherited from CivitaiModal via the manifest's superclass graph, got %+v (%v)", open, attrNames(ref.Attributes))
	}
	if size, _ := findField(ref.Attributes, "size"); size.Type != "'sm' | 'md' | 'lg'" {
		t.Errorf("ModalSize should expand from civitai-modal.d.ts, got %q", size.Type)
	}
	var evs []string
	for _, ev := range ref.Events {
		evs = append(evs, ev.Name+"@"+ev.DeclaredBy)
	}
	if got := strings.Join(evs, ","); got != "confirm@CivitaiConfirmDialog,close@CivitaiModal" {
		t.Errorf("events = %s", got)
	}
}

func TestAppElementsEventsSlotsAndParts(t *testing.T) {
	_, dir := installElementsFixture(t)
	out, _, err := runElements(t, dir, "civitai-modal")
	if err != nil {
		t.Fatalf("modal: %v", err)
	}
	for _, want := range []string{
		"\nEvents:\n  close\n",
		"\nSlots:\n  (default)\n",
		"\nCSS parts:\n  dialog\n  panel\n",
		"  heading                 string              = ''  — Not `title`: that attribute is global and would render a tooltip.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("modal output lacks %q:\n%s", want, out)
		}
	}
	// Events a base class dispatches by literal name are read from its source.
	img, _, err := runElements(t, dir, "civitai-image")
	if err != nil {
		t.Fatalf("image: %v", err)
	}
	if !strings.Contains(img, "\nEvents:\n  open\n  error  (CivitaiMediaElement)\n  load  (CivitaiMediaElement)\n") {
		t.Errorf("image should list its own `open` then the media base's `error`/`load`:\n%s", img)
	}
	if !strings.Contains(img, "  src ") || !strings.Contains(img, "  alt ") {
		t.Errorf("image should inherit src/alt from CivitaiMediaElement:\n%s", img)
	}
}

func TestAppElementsAcceptsAngleBracketTag(t *testing.T) {
	_, dir := installElementsFixture(t)
	plain, _, err := runElements(t, dir, "civitai-button")
	if err != nil {
		t.Fatalf("civitai-button: %v", err)
	}
	for _, arg := range []string{"<civitai-button>", "  <CIVITAI-BUTTON/> ", "button"} {
		got, _, err := runElements(t, dir, arg)
		if err != nil {
			t.Fatalf("%q: %v", arg, err)
		}
		if got != plain {
			t.Errorf("%q should print the same reference as civitai-button:\n%s", arg, got)
		}
	}
	// Button's `color` type is `Intent | ''`, and Intent is IMPORTED into
	// civitai-button.d.ts from civitai-badge.d.ts — the alias is followed there.
	if !strings.Contains(plain, "'info' | 'success' | 'warning' | 'error' | ''") {
		t.Errorf("Intent should expand through the import:\n%s", plain)
	}
	if !strings.Contains(plain, "\nSlots:\n  (default)\n  left\n  right\n") {
		t.Errorf("button slots:\n%s", plain)
	}
}

func TestAppElementsUnknownTagSuggests(t *testing.T) {
	_, dir := installElementsFixture(t)
	_, _, err := runElements(t, dir, "civitai-buton")
	if err == nil {
		t.Fatal("an unknown tag must fail")
	}
	if !errors.Is(err, ErrUsage) {
		t.Errorf("an unknown tag is a usage error (exit 2), got untagged: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "no element <civitai-buton>") || !strings.Contains(msg, "did you mean: <civitai-button>") {
		t.Errorf("error should name the tag and lead with the nearest match: %s", msg)
	}
	// Nothing close: no suggestion clause, still the pointer at the list.
	_, _, err = runElements(t, dir, "zzzzzzzzzz")
	if err == nil || strings.Contains(err.Error(), "did you mean") || !strings.Contains(err.Error(), "civitai app elements") {
		t.Errorf("a far-off tag should fail without suggestions and name the list command: %v", err)
	}
}

func TestAppElementsManifestNotFound(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runElements(t, dir, "civitai-button")
	if err == nil {
		t.Fatal("no installed package must fail")
	}
	if errors.Is(err, ErrUsage) {
		t.Errorf("a missing install is not a usage error: %v", err)
	}
	for _, want := range []string{"custom-elements.json", "npm install"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

// TestAppElementsReportsAnUnreadableBase: a base the reader cannot find is
// named as unresolved, never silently dropped — otherwise a gap in the reader
// would print an element that LOOKS complete.
func TestAppElementsReportsAnUnreadableBase(t *testing.T) {
	pkgDir, dir := installElementsFixture(t)
	if err := os.Remove(filepath.Join(pkgDir, "dist", "elements", "field-base.js")); err != nil {
		t.Fatal(err)
	}
	out, _, err := runElements(t, dir, "civitai-text-input", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Element elementReference `json:"element"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Element.Unresolved, ",") != "CivitaiField" {
		t.Errorf("unresolved = %v, want [CivitaiField]", got.Element.Unresolved)
	}
	human, _, _ := runElements(t, dir, "civitai-text-input")
	if !strings.Contains(human, "Incomplete: members inherited from CivitaiField could not be read") {
		t.Errorf("human output should warn that the reference is incomplete:\n%s", human)
	}
}

func TestAppElementsJSONShape(t *testing.T) {
	_, dir := installElementsFixture(t)
	out, _, err := runElements(t, dir, "civitai-checkbox", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got := raw["package"]; got["name"] != "@civitai/components" || got["version"] != "0.10.0" {
		t.Errorf("package = %v", got)
	}
	el := raw["element"]
	for _, key := range []string{"tag", "class", "description", "module", "extends", "attributes", "properties",
		"events", "slots", "cssParts", "cssProperties", "unresolved"} {
		if _, ok := el[key]; !ok {
			t.Errorf("element JSON lacks %q", key)
		}
	}
	// Every list is an array, never null — a consumer can range without a guard.
	for _, key := range []string{"extends", "attributes", "properties", "events", "slots", "cssParts", "cssProperties", "unresolved"} {
		if _, ok := el[key].([]any); !ok {
			t.Errorf("%q should be a JSON array, got %T", key, el[key])
		}
	}
	attrs := el["attributes"].([]any)
	first := attrs[0].(map[string]any)
	for _, key := range []string{"name", "type", "default", "description", "declaredBy"} {
		if _, ok := first[key]; !ok {
			t.Errorf("attribute JSON lacks %q: %v", key, first)
		}
	}

	list, _, err := runElements(t, dir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var l struct {
		Package  map[string]string `json:"package"`
		Elements []struct {
			Tag, Class, Description string
		} `json:"elements"`
	}
	if err := json.Unmarshal([]byte(list), &l); err != nil {
		t.Fatal(err)
	}
	if len(l.Elements) != 7 || l.Elements[0].Tag != "civitai-button" || l.Elements[0].Class != "CivitaiButton" {
		t.Errorf("list JSON = %+v", l.Elements)
	}
}

// TestAppElementsNearestInstallWins: Node resolves the NEAREST node_modules,
// and so must this, or a nested project is described by its parent's version.
func TestAppElementsNearestInstallWins(t *testing.T) {
	pkgDir, dir := installElementsFixture(t)
	inner := filepath.Join(dir, "nested")
	innerPkg := filepath.Join(inner, "node_modules", "@civitai", "components")
	if err := os.MkdirAll(innerPkg, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"custom-elements.json"} {
		b, err := os.ReadFile(filepath.Join(pkgDir, f))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(innerPkg, f), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(innerPkg, "package.json"), []byte(`{"name":"@civitai/components","version":"9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runElements(t, inner)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "@civitai/components 9.9.9") {
		t.Errorf("the nearest install should be read:\n%s", out)
	}
}

// TestAppElementsTerminalOutputCannotCarryEscapes: every string this command
// prints came out of a third-party npm package. A manifest whose description,
// attribute, event, slot, part or class name carries a terminal escape must not
// get it to the screen of the agent or person reading the reference — in the
// list, in one element's reference, or in the package header.
func TestAppElementsTerminalOutputCannotCarryEscapes(t *testing.T) {
	pkgDir, dir := installElementsFixture(t)
	const (
		csi = "\x1b[31m"               // recolours the rest of the screen
		osc = "\x1b]52;c;QUFBQQ==\x07" // writes the clipboard
	)
	mPath := filepath.Join(pkgDir, "custom-elements.json")
	b, err := os.ReadFile(mPath)
	if err != nil {
		t.Fatal(err)
	}
	var m cemManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	hostileModal := "CivitaiModal" + csi
	for mi := range m.Modules {
		for di := range m.Modules[mi].Declarations {
			d := &m.Modules[mi].Declarations[di]
			switch d.TagName {
			case "civitai-modal":
				d.Name = hostileModal
				d.Description = "Modal" + osc + " body. Second sentence."
				d.Attributes[0].Description = "opens" + csi
				d.Attributes[0].Default = "false" + osc
				d.Attributes[0].Type = &cemType{Text: "boolean" + csi}
				d.Attributes[0].Name = "open" + csi
				d.Events[0].Name = "close" + osc
				d.Slots[0].Name = "slot" + csi
				d.CSSParts[0].Name = "dialog" + osc
				d.CSSProperties = []cemCSSProp{{Name: "--x" + csi, Default: "1" + osc, Description: "d" + csi}}
			case "civitai-confirm-dialog":
				d.Superclass.Name = hostileModal
				for i := range d.Attributes {
					if d.Attributes[i].InheritedFrom != nil {
						d.Attributes[i].InheritedFrom.Name = hostileModal
					}
				}
			}
		}
	}
	nb, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mPath, nb, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"),
		[]byte(`{"name":"@civitai/components","version":"0.10.0\u001b[2J"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{nil, {"civitai-modal"}, {"civitai-confirm-dialog"}} {
		out, _, err := runElements(t, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.ContainsAny(out, "\x1b\x07") {
			t.Errorf("app elements %v printed a raw escape:\n%q", args, out)
		}
		// Positive control: the text around the escapes still arrives, so a
		// renderer that printed NOTHING would not pass as "clean".
		if !strings.Contains(out, "0.10.0") {
			t.Errorf("app elements %v lost the package header:\n%s", args, out)
		}
	}
	modal, _, _ := runElements(t, dir, "civitai-modal")
	for _, want := range []string{"Modal", "opens", "close", "dialog", "--x"} {
		if !strings.Contains(modal, want) {
			t.Errorf("hostile modal reference lost %q:\n%s", want, modal)
		}
	}
	list, _, _ := runElements(t, dir)
	if !strings.Contains(list, "Modal") {
		t.Errorf("list lost the description around the escape:\n%s", list)
	}
}
