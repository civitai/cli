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

// Two fixtures, both TRIMMED copies of a published @civitai/components
// manifest — class declarations only, attribute/event/slot/part entries kept,
// members reduced to public fields with no attribute:
//
//   - testdata/elements-pkg: 0.10.1, the first release whose manifest
//     describes the base classes (base.ts, field-base.ts, media-base.ts).
//     Seven elements plus the CivitaiElement and CivitaiField declarations;
//     inherited members are flattened onto each element with `inheritedFrom`,
//     and <civitai-tooltip>'s superclass is the `global:` HTMLElement root.
//   - testdata/elements-pkg-0.10.0: the published 0.10.0 manifest, which does
//     not describe the base classes. <civitai-text-input> and <civitai-button>
//     only.

// elementsTestdata is resolved once, at package init, so a test can install a
// second fixture after runElements has changed directory.
var elementsTestdata, _ = filepath.Abs("testdata")

func installElementsFixture(t *testing.T, fixture string) (pkgDir, workDir string) {
	t.Helper()
	proj := filepath.Join(t.TempDir(), "proj")
	pkgDir = filepath.Join(proj, "node_modules", "@civitai", "components")
	src := filepath.Join(elementsTestdata, fixture)
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

// editManifest rewrites the installed fixture's manifest through fn.
func editManifest(t *testing.T, pkgDir string, fn func(d *cemDecl)) {
	t.Helper()
	mPath := filepath.Join(pkgDir, "custom-elements.json")
	b, err := os.ReadFile(mPath)
	if err != nil {
		t.Fatal(err)
	}
	var m cemManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for mi := range m.Modules {
		for di := range m.Modules[mi].Declarations {
			fn(&m.Modules[mi].Declarations[di])
		}
	}
	nb, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mPath, nb, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAppElementsListsEveryTag(t *testing.T) {
	_, dir := installElementsFixture(t, "elements-pkg")
	out, _, err := runElements(t, dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "@civitai/components 0.10.1 — 7 elements") {
		t.Errorf("list header should name the package, its version and the count:\n%s", out)
	}
	for _, tag := range []string{"civitai-button", "civitai-checkbox", "civitai-modal", "civitai-sign-in-button",
		"civitai-switch", "civitai-text-input", "civitai-tooltip"} {
		if !strings.Contains(out, "\n  "+tag) {
			t.Errorf("list is missing %s:\n%s", tag, out)
		}
	}
	// Base classes are declarations, not elements: no tag, no row.
	if strings.Contains(out, "CivitaiField") || strings.Contains(out, "CivitaiElement") {
		t.Errorf("a base class is listed as an element:\n%s", out)
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
// command exists: <civitai-text-input> is used with label/name/value/…, which
// it inherits from CivitaiField.
func TestAppElementsFormControlIncludesInheritedAttributes(t *testing.T) {
	_, dir := installElementsFixture(t, "elements-pkg")
	out, _, err := runElements(t, dir, "civitai-text-input")
	if err != nil {
		t.Fatalf("text-input: %v", err)
	}
	if !strings.Contains(out, "<civitai-text-input>  CivitaiTextInput → CivitaiField → CivitaiElement → LitElement\n") {
		t.Errorf("header should show the superclass chain:\n%s", out)
	}
	for _, row := range []string{
		"  maxlength     number     = 0  — 0 means unbounded, matching an absent native `maxlength`.\n",
		"  label         string     = ''  (CivitaiField)\n",
		"  required      boolean    = false  (CivitaiField)\n",
		"  size          FieldSize  = 'md'  (CivitaiField)\n",
		"  error         string     = ''  — Message to show; also marks the control invalid.  (CivitaiField)\n",
		"  change  — A `change` from inside the shadow root",
	} {
		if !strings.Contains(out, row) {
			t.Errorf("row missing or malformed:\nwant: %q\n%s", row, out)
		}
	}
	for _, name := range []string{"placeholder", "readonly", "autocomplete", "name", "value", "description", "disabled"} {
		if !strings.Contains(out, "\n  "+name+" ") {
			t.Errorf("attribute %q missing:\n%s", name, out)
		}
	}
	if strings.Contains(out, "Incomplete") {
		t.Errorf("a fully described chain must not be reported incomplete:\n%s", out)
	}
	if len(out) > 3000 {
		t.Errorf("one form control's reference is %d bytes; it should stay well under 3 KB", len(out))
	}
}

// TestAppElementsInheritedMembersAndChainComeFromTheManifest: members come
// from the element's own entry, where the analyzer flattened the inherited ones
// with `inheritedFrom`; the superclass walk supplies the CHAIN (`extends`) and
// nothing else. A chain through another element — switch → checkbox →
// CivitaiField — shows checkbox's `value` override once, not CivitaiField's too.
func TestAppElementsInheritedMembersAndChainComeFromTheManifest(t *testing.T) {
	pkgDir, _ := installElementsFixture(t, "elements-pkg")
	pkg, err := loadElementsPackage(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	ti, _ := pkg.lookupElement("civitai-text-input")
	ref := pkg.resolveElement(ti)
	want := "placeholder,type,readonly,maxlength,autocomplete,name,value,label,description,error,required,disabled,size"
	if names := strings.Join(attrNames(ref.Attributes), ","); names != want {
		t.Errorf("text-input attributes:\n got %s\nwant %s", names, want)
	}
	if label, _ := findField(ref.Attributes, "label"); label.DeclaredBy != "CivitaiField" {
		t.Errorf("`label` should be declared by CivitaiField, got %+v", label)
	}
	if chain := strings.Join(ref.Extends, ","); chain != "CivitaiField,CivitaiElement,LitElement" {
		t.Errorf("text-input chain = %s", chain)
	}

	sw, _ := pkg.lookupElement("civitai-switch")
	sref := pkg.resolveElement(sw)
	if chain := strings.Join(sref.Extends, ","); chain != "CivitaiCheckbox,CivitaiField,CivitaiElement,LitElement" {
		t.Errorf("switch chain = %s", chain)
	}
	if names := strings.Join(attrNames(sref.Attributes), ","); names != "value,checked,indeterminate,name,label,description,error,required,disabled,size" {
		t.Errorf("switch attributes = %s", names)
	}
	if v, _ := findField(sref.Attributes, "value"); v.Default != "'on'" || v.DeclaredBy != "CivitaiCheckbox" {
		t.Errorf("switch `value` should be checkbox's override ('on'), got %+v", v)
	}
	if len(sref.Unresolved) != 0 {
		t.Errorf("unresolved = %v", sref.Unresolved)
	}
}

// TestAppElementsPropertiesAreThePublicAPIOnly: the published manifest lists
// protected, private, static and attribute-backed members too (the fixture is
// trimmed of them, so they are put back here). None is something an author
// sets on the element, so none may reach the property list.
func TestAppElementsPropertiesAreThePublicAPIOnly(t *testing.T) {
	pkgDir, _ := installElementsFixture(t, "elements-pkg")
	editManifest(t, pkgDir, func(d *cemDecl) {
		if d.TagName == "civitai-text-input" {
			d.Members = append(d.Members,
				cemMember{Kind: "field", Name: "internals", Privacy: "protected"},
				cemMember{Kind: "field", Name: "secret", Privacy: "private"},
				cemMember{Kind: "field", Name: "#onInput"},
				cemMember{Kind: "field", Name: "formAssociated", Static: true},
				cemMember{Kind: "field", Name: "placeholder", Attribute: "placeholder"},
				cemMember{Kind: "method", Name: "checkValidity"},
			)
		}
	})
	pkg, err := loadElementsPackage(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := pkg.lookupElement("civitai-text-input")
	if got := strings.Join(attrNames(pkg.resolveElement(e).Properties), ","); got != "form,validity" {
		t.Errorf("properties = %s, want form,validity", got)
	}
}

// TestAppElementsResolvesASuperclassByModulePathOnly: a superclass reference
// resolves only where its MODULE is in the manifest. The same class name under
// a module the manifest does not contain (a re-export barrel) is unresolved —
// reported, not guessed at by name.
func TestAppElementsResolvesASuperclassByModulePathOnly(t *testing.T) {
	pkgDir, _ := installElementsFixture(t, "elements-pkg")
	editManifest(t, pkgDir, func(d *cemDecl) {
		if d.TagName == "civitai-button" {
			d.Superclass = &cemRef{Name: "CivitaiElement", Module: "/src/elements/internals.js"}
		}
	})
	pkg, err := loadElementsPackage(pkgDir)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := pkg.lookupElement("civitai-button")
	if got := strings.Join(pkg.resolveElement(e).Unresolved, ","); got != "CivitaiElement" {
		t.Errorf("a superclass under a module the manifest lacks must be unresolved, got %q", got)
	}
	// Control: the same name under its real module resolves.
	m, _ := pkg.lookupElement("civitai-modal")
	if got := pkg.resolveElement(m); len(got.Unresolved) != 0 || strings.Join(got.Extends, ",") != "CivitaiElement,LitElement" {
		t.Errorf("modal: extends %v, unresolved %v", got.Extends, got.Unresolved)
	}
}

// TestAppElementsAGlobalOrFrameworkRootIsNotIncomplete: a light-DOM element
// extends the platform's HTMLElement and a Lit one ends at LitElement. Neither
// is in this package's manifest and neither is missing anything, so neither
// may produce the "Incomplete" warning.
func TestAppElementsAGlobalOrFrameworkRootIsNotIncomplete(t *testing.T) {
	pkgDir, dir := installElementsFixture(t, "elements-pkg")
	for _, tag := range []string{"civitai-tooltip", "civitai-button"} {
		out, _, err := runElements(t, dir, tag, "--json")
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Element elementReference `json:"element"`
		}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Element.Unresolved) != 0 {
			t.Errorf("%s: a root superclass was reported unresolved: %v", tag, got.Element.Unresolved)
		}
		human, _, _ := runElements(t, dir, tag)
		if strings.Contains(human, "Incomplete") {
			t.Errorf("%s printed Incomplete:\n%s", tag, human)
		}
	}
	tip, _, _ := runElements(t, dir, "civitai-tooltip")
	if !strings.Contains(tip, "<civitai-tooltip>  CivitaiTooltip → HTMLElement\n") {
		t.Errorf("tooltip chain should end at HTMLElement:\n%s", tip)
	}

	// Any `package`-qualified class is a root, whatever its name.
	resolveTooltip := func(sc cemRef) elementReference {
		t.Helper()
		editManifest(t, pkgDir, func(d *cemDecl) {
			if d.TagName == "civitai-tooltip" {
				r := sc
				d.Superclass = &r
			}
		})
		pkg, err := loadElementsPackage(pkgDir)
		if err != nil {
			t.Fatal(err)
		}
		e, _ := pkg.lookupElement("civitai-tooltip")
		return pkg.resolveElement(e)
	}
	if ref := resolveTooltip(cemRef{Name: "SomeFrameworkBase", Package: "some-framework"}); len(ref.Unresolved) != 0 {
		t.Errorf("a package-qualified superclass was reported unresolved: %v", ref.Unresolved)
	}
	// Controls: a root is recognised by its `package`, never by its name, and
	// an undeclared non-root is unresolved.
	for _, sc := range []cemRef{{Name: "HTMLElement"}, {Name: "HTMLElementBase", Module: "/src/elements/html-element.js"}} {
		if ref := resolveTooltip(sc); strings.Join(ref.Unresolved, ",") != sc.Name {
			t.Errorf("%+v must be unresolved, got %v", sc, ref.Unresolved)
		}
	}
}

// TestAppElementsReportsAnOldManifestAsIncomplete: 0.10.0 does not describe the
// base classes. The reference is printed with what the manifest has, and says
// it is incomplete, names the class, and names the fix — no fallback parsing.
func TestAppElementsReportsAnOldManifestAsIncomplete(t *testing.T) {
	_, dir := installElementsFixture(t, "elements-pkg-0.10.0")
	out, _, err := runElements(t, dir, "civitai-text-input")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Incomplete: this package's custom-elements.json does not describe CivitaiField, "+
		"so members inherited from it are not listed above. Fix: upgrade @civitai/components.") {
		t.Errorf("0.10.0 text-input should be reported incomplete, naming CivitaiField and the fix:\n%s", out)
	}
	if strings.Contains(out, "  label ") {
		t.Errorf("an attribute the manifest does not carry was printed anyway:\n%s", out)
	}
	js, _, err := runElements(t, dir, "civitai-button", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Element elementReference `json:"element"`
	}
	if err := json.Unmarshal([]byte(js), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Element.Unresolved, ",") != "CivitaiElement" {
		t.Errorf("0.10.0 button unresolved = %v, want [CivitaiElement]", got.Element.Unresolved)
	}
}

func TestAppElementsEventsSlotsAndParts(t *testing.T) {
	_, dir := installElementsFixture(t, "elements-pkg")
	out, _, err := runElements(t, dir, "civitai-modal")
	if err != nil {
		t.Fatalf("modal: %v", err)
	}
	for _, want := range []string{
		"\nEvents:\n  close\n  change  — ",
		"\nSlots:\n  (default)\n",
		"\nCSS parts:\n  dialog\n  panel\n",
		"  heading                 string     = ''  — Not `title`: that attribute is global and would render a tooltip.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("modal output lacks %q:\n%s", want, out)
		}
	}
}

func TestAppElementsAcceptsAngleBracketTag(t *testing.T) {
	_, dir := installElementsFixture(t, "elements-pkg")
	plain, _, err := runElements(t, dir, "civitai-button")
	if err != nil {
		t.Fatalf("civitai-button: %v", err)
	}
	if !strings.Contains(plain, "\nSlots:\n  (default)\n  left\n  right\n") {
		t.Errorf("button slots:\n%s", plain)
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
}

func TestAppElementsUnknownTagSuggests(t *testing.T) {
	_, dir := installElementsFixture(t, "elements-pkg")
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
	_, _, err = runElements(t, dir, "zzzzzzzzzz")
	if err == nil || strings.Contains(err.Error(), "did you mean") || !strings.Contains(err.Error(), "civitai app elements") {
		t.Errorf("a far-off tag should fail without suggestions and name the list command: %v", err)
	}
}

func TestAppElementsManifestNotFound(t *testing.T) {
	_, _, err := runElements(t, t.TempDir(), "civitai-button")
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

func TestAppElementsJSONShape(t *testing.T) {
	_, dir := installElementsFixture(t, "elements-pkg")
	out, _, err := runElements(t, dir, "civitai-checkbox", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got := raw["package"]; got["name"] != "@civitai/components" || got["version"] != "0.10.1" {
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
	first := el["attributes"].([]any)[0].(map[string]any)
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
	pkgDir, dir := installElementsFixture(t, "elements-pkg")
	innerPkg := filepath.Join(dir, "nested", "node_modules", "@civitai", "components")
	if err := os.MkdirAll(innerPkg, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(pkgDir, "custom-elements.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(innerPkg, "custom-elements.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(innerPkg, "package.json"), []byte(`{"name":"@civitai/components","version":"9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runElements(t, filepath.Join(dir, "nested"))
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
	pkgDir, dir := installElementsFixture(t, "elements-pkg")
	const (
		csi = "\x1b[31m"               // recolours the rest of the screen
		osc = "\x1b]52;c;QUFBQQ==\x07" // writes the clipboard
	)
	hostileCheckbox := "CivitaiCheckbox" + csi
	editManifest(t, pkgDir, func(d *cemDecl) {
		switch d.TagName {
		case "civitai-modal":
			d.Description = "Modal" + osc + " body. Second sentence."
			d.Attributes[0].Description = "opens" + csi
			d.Attributes[0].Default = "false" + osc
			d.Attributes[0].Type = &cemType{Text: "boolean" + csi}
			d.Attributes[0].Name = "open" + csi
			d.Events[0].Name = "close" + osc
			d.Slots[0].Name = "slot" + csi
			d.CSSParts[0].Name = "dialog" + osc
			d.CSSProperties = []cemCSSProp{{Name: "--x" + csi, Default: "1" + osc, Description: "d" + csi}}
		case "civitai-checkbox":
			d.Name = hostileCheckbox
		case "civitai-switch":
			d.Superclass.Name = hostileCheckbox
			for i := range d.Attributes {
				if d.Attributes[i].InheritedFrom != nil {
					d.Attributes[i].InheritedFrom.Name = hostileCheckbox
				}
			}
		}
	})
	if err := os.WriteFile(filepath.Join(pkgDir, "package.json"),
		[]byte(`{"name":"@civitai/components","version":"0.10.1\u001b[2J"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{nil, {"civitai-modal"}, {"civitai-switch"}} {
		out, _, err := runElements(t, dir, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.ContainsAny(out, "\x1b\x07") {
			t.Errorf("app elements %v printed a raw escape:\n%q", args, out)
		}
		// Positive control: the text around the escapes still arrives, so a
		// renderer that printed NOTHING would not pass as "clean".
		if !strings.Contains(out, "0.10.1") {
			t.Errorf("app elements %v lost the package header:\n%s", args, out)
		}
	}
	modal, _, _ := runElements(t, dir, "civitai-modal")
	for _, want := range []string{"Modal", "opens", "close", "dialog", "--x"} {
		if !strings.Contains(modal, want) {
			t.Errorf("hostile modal reference lost %q:\n%s", want, modal)
		}
	}
	sw, _, _ := runElements(t, dir, "civitai-switch")
	if !strings.Contains(sw, "CivitaiCheckbox") {
		t.Errorf("hostile switch reference lost its superclass name:\n%s", sw)
	}

	// The unknown-tag ERROR echoes package bytes too — the package name and
	// version, and the near-match tag names — and main prints err.Error() raw.
	// A payload that sets the terminal title and clears the screen must not
	// survive into the error or onto stderr.
	const payload = "\x1b]0;PWNED\x07\x1b[2J"
	pkgDir2, dir2 := installElementsFixture(t, "elements-pkg")
	editManifest(t, pkgDir2, func(d *cemDecl) {
		if d.TagName == "civitai-tooltip" {
			d.TagName = "civitai-tooltip" + payload
		}
	})
	if err := os.WriteFile(filepath.Join(pkgDir2, "package.json"),
		[]byte(`{"name":"@civitai/components`+"\\u001b[2J"+`","version":"0.10.1\u001b]0;PWNED\u0007\u001b[2J"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, err := runElements(t, dir2, "civitai-toolti")
	if err == nil {
		t.Fatal("unknown tag must fail")
	}
	if strings.ContainsAny(err.Error(), "\x1b\x07") || strings.ContainsAny(stderr, "\x1b\x07") {
		t.Errorf("the unknown-tag error carries a raw escape:\nerr: %q\nstderr: %q", err.Error(), stderr)
	}
	// Positive control: the sanitised error still names the package version and
	// the near match, so a message that dropped them would not pass.
	if !strings.Contains(err.Error(), "0.10.1") || !strings.Contains(err.Error(), "<civitai-tooltip") {
		t.Errorf("the unknown-tag error lost the version or the near match: %q", err.Error())
	}

	// The Incomplete warning names a superclass FROM the package, and a
	// manifest that does not parse produces an error main prints raw. Neither
	// may carry the payload.
	pkgDir3, dir3 := installElementsFixture(t, "elements-pkg-0.10.0")
	editManifest(t, pkgDir3, func(d *cemDecl) {
		if d.TagName == "civitai-text-input" {
			d.Superclass.Name = "CivitaiField" + payload
		}
	})
	out3, _, err := runElements(t, dir3, "civitai-text-input")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out3, "\x1b\x07") || !strings.Contains(out3, "Incomplete: this package's custom-elements.json does not describe CivitaiField") {
		t.Errorf("the Incomplete warning should name the class without its escapes:\n%q", out3)
	}
	if err := os.WriteFile(filepath.Join(pkgDir3, "custom-elements.json"), []byte("{"+payload), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr3, err := runElements(t, dir3, "civitai-text-input")
	if err == nil {
		t.Fatal("a manifest that does not parse must fail")
	}
	if strings.ContainsAny(err.Error(), "\x1b\x07") || strings.ContainsAny(stderr3, "\x1b\x07") {
		t.Errorf("the parse error carries a raw escape: %q", err.Error())
	}

}
