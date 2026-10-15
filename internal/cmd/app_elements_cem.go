package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// This file reads the Custom Elements Manifest (CEM) that `@civitai/components`
// publishes, resolves one element's FULL member set — its own plus everything it
// inherits — and nothing else. `app_elements.go` is the command that prints it.
//
// 🔴 WHY THERE IS A SOURCE READER HERE AND NOT JUST A JSON READER. The manifest
// describes every module the analyzer was pointed at, and the analyzer was
// pointed at the element files only. So an element whose superclass is ANOTHER
// element (`<civitai-switch>` → `CivitaiCheckbox`, `<civitai-confirm-dialog>` →
// `CivitaiModal`) arrives flattened, each inherited member carrying
// `inheritedFrom`; but an element whose superclass is a shared BASE that is not
// itself an element (`CivitaiField`, `CivitaiMediaElement`) arrives with only
// its own members and a `superclass` reference to a module the manifest does not
// contain. Measured on @civitai/components 0.10.0: 46 modules, none of them the
// base modules, so `<civitai-text-input>` lists `placeholder`, `type`,
// `readonly`, `maxlength`, `autocomplete` — and not `label`, `name`, `value`,
// `description`, `error`, `required`, `disabled` or `size`, the attributes every
// form control is used with.
//
// Those members are not invented here. They are read from the base class's
// PUBLISHED compiled output in the same installed package — the `.js` file's Lit
// `static properties` (which property is reactive, its attribute name, its
// type converter), its constructor (the default), and the sibling `.d.ts` (the
// declared type and its doc comment). Nothing about any element is hard-coded
// in Go; a base the reader cannot find or parse is REPORTED as unresolved,
// never silently dropped, so a gap in this reader shows up in the output rather
// than as an element that looks complete.

// cemManifest is the subset of the Custom Elements Manifest schema
// (https://github.com/webcomponents/custom-elements-manifest) this command reads.
type cemManifest struct {
	SchemaVersion string      `json:"schemaVersion"`
	Modules       []cemModule `json:"modules"`
}

type cemModule struct {
	Path         string    `json:"path"`
	Declarations []cemDecl `json:"declarations"`
}

type cemDecl struct {
	Kind          string       `json:"kind"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	Summary       string       `json:"summary"`
	TagName       string       `json:"tagName"`
	CustomElement bool         `json:"customElement"`
	Superclass    *cemRef      `json:"superclass"`
	Members       []cemMember  `json:"members"`
	Attributes    []cemAttr    `json:"attributes"`
	Events        []cemEvent   `json:"events"`
	Slots         []cemNamed   `json:"slots"`
	CSSParts      []cemNamed   `json:"cssParts"`
	CSSProperties []cemCSSProp `json:"cssProperties"`
}

type cemRef struct {
	Name    string `json:"name"`
	Module  string `json:"module"`
	Package string `json:"package"`
}

type cemType struct {
	Text string `json:"text"`
}

type cemMember struct {
	Kind          string   `json:"kind"`
	Name          string   `json:"name"`
	Privacy       string   `json:"privacy"`
	Static        bool     `json:"static"`
	Readonly      bool     `json:"readonly"`
	Description   string   `json:"description"`
	Default       string   `json:"default"`
	Attribute     string   `json:"attribute"`
	Type          *cemType `json:"type"`
	InheritedFrom *cemRef  `json:"inheritedFrom"`
}

type cemAttr struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Default       string   `json:"default"`
	FieldName     string   `json:"fieldName"`
	Type          *cemType `json:"type"`
	InheritedFrom *cemRef  `json:"inheritedFrom"`
}

type cemEvent struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Type          *cemType `json:"type"`
	InheritedFrom *cemRef  `json:"inheritedFrom"`
}

type cemNamed struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	InheritedFrom *cemRef `json:"inheritedFrom"`
}

type cemCSSProp struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     string `json:"default"`
	Syntax      string `json:"syntax"`
}

// elementsPackageName is the package whose manifest this command reads.
const elementsPackageName = "@civitai/components"

// elementsPackage is one installed copy of @civitai/components.
type elementsPackage struct {
	Dir          string // the package root inside node_modules
	Name         string
	Version      string
	ManifestPath string
	Manifest     cemManifest
}

// errElementsManifestNotFound is returned when no installed copy is reachable
// from the start directory.
var errElementsManifestNotFound = errors.New("custom elements manifest not found")

// findElementsPackage walks up from start to the nearest
// node_modules/@civitai/components that carries a manifest — the same lookup
// Node's resolver performs for a bare import from start, so the copy described
// is the one the project's own code imports.
func findElementsPackage(start string) (*elementsPackage, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	for {
		pkgDir := filepath.Join(dir, "node_modules", filepath.FromSlash(elementsPackageName))
		if st, err := os.Stat(pkgDir); err == nil && st.IsDir() {
			pkg, err := loadElementsPackage(pkgDir)
			if err == nil {
				return pkg, nil
			}
			if !errors.Is(err, errElementsManifestNotFound) {
				return nil, err
			}
			// A copy without a manifest (a version predating it) is skipped, as
			// Node would still resolve it — but say so if nothing better is found.
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, errElementsManifestNotFound
		}
		dir = parent
	}
}

// loadElementsPackage reads the package.json and the manifest it names. The
// CEM convention is a `customElements` key in package.json; `custom-elements.json`
// at the package root is the fallback every CEM analyzer writes by default.
func loadElementsPackage(pkgDir string) (*elementsPackage, error) {
	var meta struct {
		Name           string `json:"name"`
		Version        string `json:"version"`
		CustomElements string `json:"customElements"`
	}
	if b, err := os.ReadFile(filepath.Join(pkgDir, "package.json")); err == nil {
		if err := json.Unmarshal(b, &meta); err != nil {
			return nil, fmt.Errorf("read %s: %w", filepath.Join(pkgDir, "package.json"), err)
		}
	}
	rel := meta.CustomElements
	if rel == "" {
		rel = "custom-elements.json"
	}
	manifestPath := filepath.Join(pkgDir, filepath.FromSlash(rel))
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errElementsManifestNotFound
		}
		return nil, err
	}
	var m cemManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", manifestPath, err)
	}
	name := meta.Name
	if name == "" {
		name = elementsPackageName
	}
	return &elementsPackage{Dir: pkgDir, Name: name, Version: meta.Version, ManifestPath: manifestPath, Manifest: m}, nil
}

// elementDecl is a custom-element class declaration and the module holding it.
type elementDecl struct {
	Module string
	Decl   cemDecl
}

// elements returns every declaration with a tag name, sorted by tag.
func (p *elementsPackage) elements() []elementDecl {
	var out []elementDecl
	for _, m := range p.Manifest.Modules {
		for _, d := range m.Declarations {
			if d.TagName != "" {
				out = append(out, elementDecl{Module: m.Path, Decl: d})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Decl.TagName < out[j].Decl.TagName })
	return out
}

// moduleKey normalises a module reference so the manifest's two spellings of
// one module compare equal: a declaration's own `path` is the SOURCE file
// (`src/elements/civitai-modal.ts`) while a `superclass.module` reference to it
// is the import specifier (`/src/elements/civitai-modal.js`).
func moduleKey(p string) string {
	p = strings.TrimPrefix(path.Clean("/"+strings.TrimSpace(p)), "/")
	for _, ext := range []string{".d.ts", ".ts", ".js", ".mjs"} {
		if strings.HasSuffix(p, ext) {
			return strings.TrimSuffix(p, ext)
		}
	}
	return p
}

// findDecl finds a class declaration in the manifest by reference.
func (p *elementsPackage) findDecl(ref cemRef) (elementDecl, bool) {
	want := moduleKey(ref.Module)
	var byName []elementDecl
	for _, m := range p.Manifest.Modules {
		for _, d := range m.Declarations {
			if d.Kind != "class" || d.Name != ref.Name {
				continue
			}
			if ref.Module != "" && moduleKey(m.Path) == want {
				return elementDecl{Module: m.Path, Decl: d}, true
			}
			byName = append(byName, elementDecl{Module: m.Path, Decl: d})
		}
	}
	if ref.Module == "" && len(byName) == 1 {
		return byName[0], true
	}
	return elementDecl{}, false
}

// compiledFile maps a manifest module path (a SOURCE path) to the published
// file with the given extension. The path is tried as written first; then with
// its leading `src/` replaced by `dist/` — the TypeScript rootDir→outDir layout
// @civitai/components is published with, where the manifest's
// `src/elements/civitai-modal.ts` ships as `dist/elements/civitai-modal.js`.
// It returns "" when neither exists.
func (p *elementsPackage) compiledFile(module, ext string) string {
	key := moduleKey(module)
	cands := []string{key + ext}
	if rest, ok := strings.CutPrefix(key, "src/"); ok {
		cands = append(cands, "dist/"+rest+ext)
	}
	for _, c := range cands {
		f := filepath.Join(p.Dir, filepath.FromSlash(c))
		if st, err := os.Stat(f); err == nil && !st.IsDir() {
			return f
		}
	}
	return ""
}

// elementField is one attribute or property in the resolved reference.
type elementField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
	// DeclaredBy is the class that declares the member: the element's own
	// class, or the ancestor it is inherited from.
	DeclaredBy string `json:"declaredBy"`
}

// elementNamed is an event, slot, CSS part or CSS custom property.
type elementNamed struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
	DeclaredBy  string `json:"declaredBy"`
}

// elementReference is everything one element accepts, inherited members
// included. It is the `element` object of `app elements <tag> --json`.
type elementReference struct {
	Tag         string `json:"tag"`
	Class       string `json:"class"`
	Description string `json:"description"`
	Module      string `json:"module"`
	// Extends is the superclass chain, nearest first, as far as it could be
	// followed inside the package.
	Extends       []string       `json:"extends"`
	Attributes    []elementField `json:"attributes"`
	Properties    []elementField `json:"properties"`
	Events        []elementNamed `json:"events"`
	Slots         []elementNamed `json:"slots"`
	CSSParts      []elementNamed `json:"cssParts"`
	CSSProperties []elementNamed `json:"cssProperties"`
	// Unresolved names each superclass whose members could not be read, so an
	// incomplete answer says so instead of looking complete.
	Unresolved []string `json:"unresolved"`
}

// referenceBuilder accumulates members nearest-class-first; the first
// declaration of a name wins, so an override shadows what it overrides.
type referenceBuilder struct {
	ref                                     elementReference
	attrs, props, events, slots, parts, css map[string]bool
}

func newReferenceBuilder(e elementDecl) *referenceBuilder {
	b := &referenceBuilder{
		attrs: map[string]bool{}, props: map[string]bool{}, events: map[string]bool{},
		slots: map[string]bool{}, parts: map[string]bool{}, css: map[string]bool{},
	}
	b.ref = elementReference{
		Tag: e.Decl.TagName, Class: e.Decl.Name, Module: e.Module,
		Description: firstNonEmpty(e.Decl.Summary, e.Decl.Description),
		Extends:     []string{}, Attributes: []elementField{}, Properties: []elementField{},
		Events: []elementNamed{}, Slots: []elementNamed{}, CSSParts: []elementNamed{},
		CSSProperties: []elementNamed{}, Unresolved: []string{},
	}
	return b
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func typeText(t *cemType) string {
	if t == nil {
		return ""
	}
	return t.Text
}

func declaredBy(inh *cemRef, own string) string {
	if inh != nil && inh.Name != "" {
		return inh.Name
	}
	return own
}

func (b *referenceBuilder) addAttr(f elementField) {
	if f.Name == "" || b.attrs[f.Name] {
		return
	}
	b.attrs[f.Name] = true
	b.ref.Attributes = append(b.ref.Attributes, f)
}

func (b *referenceBuilder) addProp(f elementField) {
	if f.Name == "" || b.props[f.Name] {
		return
	}
	b.props[f.Name] = true
	b.ref.Properties = append(b.ref.Properties, f)
}

func addNamed(seen map[string]bool, list *[]elementNamed, n elementNamed) {
	key := n.Name // the default slot's name is "" and is a real, distinct slot
	if seen[key] {
		return
	}
	seen[key] = true
	*list = append(*list, n)
}

// addManifestDecl adds one manifest class declaration's members.
func (b *referenceBuilder) addManifestDecl(d cemDecl) {
	// Properties whose attribute is listed are already covered by that
	// attribute; mark their field names so the property list stays "distinct".
	attrField := map[string]bool{}
	for _, a := range d.Attributes {
		b.addAttr(elementField{
			Name: a.Name, Type: typeText(a.Type), Default: a.Default,
			Description: strings.TrimSpace(a.Description), DeclaredBy: declaredBy(a.InheritedFrom, d.Name),
		})
		if a.FieldName != "" {
			attrField[a.FieldName] = true
		}
	}
	for _, m := range d.Members {
		if m.Kind != "field" || m.Static || m.Attribute != "" || attrField[m.Name] {
			continue
		}
		if m.Privacy == "private" || m.Privacy == "protected" || strings.HasPrefix(m.Name, "#") {
			continue
		}
		b.addProp(elementField{
			Name: m.Name, Type: typeText(m.Type), Default: m.Default,
			Description: strings.TrimSpace(m.Description), DeclaredBy: declaredBy(m.InheritedFrom, d.Name),
		})
	}
	for _, e := range d.Events {
		addNamed(b.events, &b.ref.Events, elementNamed{Name: e.Name, Type: typeText(e.Type),
			Description: strings.TrimSpace(e.Description), DeclaredBy: declaredBy(e.InheritedFrom, d.Name)})
	}
	for _, s := range d.Slots {
		addNamed(b.slots, &b.ref.Slots, elementNamed{Name: s.Name,
			Description: strings.TrimSpace(s.Description), DeclaredBy: declaredBy(s.InheritedFrom, d.Name)})
	}
	for _, s := range d.CSSParts {
		addNamed(b.parts, &b.ref.CSSParts, elementNamed{Name: s.Name,
			Description: strings.TrimSpace(s.Description), DeclaredBy: declaredBy(s.InheritedFrom, d.Name)})
	}
	for _, c := range d.CSSProperties {
		addNamed(b.css, &b.ref.CSSProperties, elementNamed{Name: c.Name, Type: c.Syntax, Default: c.Default,
			Description: strings.TrimSpace(c.Description), DeclaredBy: d.Name})
	}
}

// resolveElement builds the full reference for one element: its own members,
// then each ancestor's, following `superclass` through the manifest and, where
// the manifest stops, through the package's compiled source.
//
// followSuperclasses=false is the seam the inheritance test uses to prove the
// inherited assertions depend on this walk; production always passes true.
func (p *elementsPackage) resolveElement(e elementDecl, followSuperclasses bool) elementReference {
	b := newReferenceBuilder(e)
	b.addManifestDecl(e.Decl)
	if followSuperclasses {
		p.walkSuperclasses(b, e.Decl.Superclass)
	}
	p.expandTypes(&b.ref)
	return b.ref
}

func (p *elementsPackage) walkSuperclasses(b *referenceBuilder, ref *cemRef) {
	visited := map[string]bool{}
	for ref != nil && ref.Name != "" {
		key := ref.Name + "\x00" + moduleKey(ref.Module)
		if visited[key] {
			return
		}
		visited[key] = true
		b.ref.Extends = append(b.ref.Extends, ref.Name)
		if ref.Package != "" {
			return // a class from another package (e.g. a framework base): out of scope
		}
		if d, ok := p.findDecl(*ref); ok {
			b.addManifestDecl(d.Decl)
			ref = d.Decl.Superclass
			continue
		}
		// Not in the manifest: read it from the package's compiled output.
		src := p.compiledFile(ref.Module, ".js")
		if src == "" {
			b.ref.Unresolved = append(b.ref.Unresolved, ref.Name)
			return
		}
		next, ok := p.addSourceClass(b, src, ref.Name)
		if !ok {
			b.ref.Unresolved = append(b.ref.Unresolved, ref.Name)
			return
		}
		ref = next
	}
}

// ---- compiled-source reader -------------------------------------------------

// sourceClass is what the reader extracts from one compiled class.
type sourceClass struct {
	file       string // the .js file the class body was found in
	body       string // class body, between its braces
	superName  string // the identifier after `extends`
	superSpec  string // the import specifier it came from ("" if local/unknown)
	properties []litProperty
	defaults   map[string]string
	events     []string
}

type litProperty struct {
	Name      string
	Attribute string // "" when attribute:false
	Converter string // Lit `type:` (Boolean, Number, String, Array, Object) or ""
}

var (
	reImportNamed = regexp.MustCompile(`import\s+(?:type\s+)?\{([^}]*)\}\s*from\s*['"]([^'"]+)['"]`)
	reExportFrom  = regexp.MustCompile(`export\s+(?:type\s+)?\{([^}]*)\}\s*from\s*['"]([^'"]+)['"]`)
	reCtorAssign  = regexp.MustCompile(`this\.([A-Za-z_$][\w$]*)\s*=\s*([^;\n]+);`)
	reNewEvent    = regexp.MustCompile(`new\s+(Custom)?Event\(\s*['"]([\w:.-]+)['"]`)
	reLitType     = regexp.MustCompile(`\btype\s*:\s*(Boolean|Number|String|Array|Object)\b`)
	reLitAttrStr  = regexp.MustCompile(`\battribute\s*:\s*['"]([^'"]+)['"]`)
	reLitAttrOff  = regexp.MustCompile(`\battribute\s*:\s*false\b`)
	reLitState    = regexp.MustCompile(`\bstate\s*:\s*true\b`)
	reIdent       = regexp.MustCompile(`^[A-Za-z_$][\w$]*$`)
)

// addSourceClass reads class `name` from the compiled file `file` (following a
// re-export barrel if the file only re-exports it), adds its members to b, and
// returns its superclass reference — nil when the superclass is imported from
// another package. ok=false means the class could not be read.
func (p *elementsPackage) addSourceClass(b *referenceBuilder, file, name string) (*cemRef, bool) {
	sc, ok := readSourceClass(file, name, 0)
	if !ok {
		return nil, false
	}
	dts := p.declarationInfo(sc.file, name)
	for _, lp := range sc.properties {
		info := dts.fields[lp.Name]
		typ := info.typ
		if typ == "" {
			typ = strings.ToLower(lp.Converter)
		}
		f := elementField{Name: lp.Attribute, Type: typ, Default: sc.defaults[lp.Name],
			Description: info.doc, DeclaredBy: name}
		if lp.Attribute == "" {
			f.Name = lp.Name
			b.addProp(f)
			continue
		}
		b.addAttr(f)
		b.props[lp.Name] = true // the attribute already documents this property
	}
	// Public fields and getters the .d.ts declares that are not reactive.
	for _, fname := range dts.order {
		info := dts.fields[fname]
		if b.props[fname] || isReactive(sc.properties, fname) {
			continue
		}
		b.addProp(elementField{Name: fname, Type: info.typ, Description: info.doc, DeclaredBy: name})
	}
	for _, ev := range sc.events {
		addNamed(b.events, &b.ref.Events, elementNamed{Name: ev, Type: "Event", DeclaredBy: name})
	}
	if sc.superName == "" || sc.superSpec == "" || !isRelativeSpec(sc.superSpec) {
		return nil, true // LitElement and friends: imported from another package
	}
	next := filepath.Join(filepath.Dir(sc.file), filepath.FromSlash(sc.superSpec))
	rel, err := filepath.Rel(p.Dir, next)
	if err != nil {
		return nil, true
	}
	ref := &cemRef{Name: sc.superName, Module: filepath.ToSlash(rel)}
	return ref, true
}

func isReactive(props []litProperty, name string) bool {
	for _, lp := range props {
		if lp.Name == name {
			return true
		}
	}
	return false
}

func isRelativeSpec(s string) bool { return strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") }

// readSourceClass finds `class <name>` in file, or follows `export { name } from`
// to the file that declares it.
func readSourceClass(file, name string, depth int) (sourceClass, bool) {
	if depth > 4 {
		return sourceClass{}, false
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return sourceClass{}, false
	}
	src := string(b)
	reClass := regexp.MustCompile(`\bclass\s+` + regexp.QuoteMeta(name) + `\b(?:\s+extends\s+([A-Za-z_$][\w$.]*))?[^{]*\{`)
	loc := reClass.FindStringSubmatchIndex(src)
	if loc == nil {
		for _, m := range reExportFrom.FindAllStringSubmatch(src, -1) {
			if importListHas(m[1], name) && isRelativeSpec(m[2]) {
				return readSourceClass(filepath.Join(filepath.Dir(file), filepath.FromSlash(m[2])), name, depth+1)
			}
		}
		return sourceClass{}, false
	}
	open := loc[1] - 1
	end := matchBrace(src, open)
	if end < 0 {
		return sourceClass{}, false
	}
	sc := sourceClass{file: file, body: src[open+1 : end], defaults: map[string]string{}}
	if loc[2] >= 0 {
		sc.superName = src[loc[2]:loc[3]]
		for _, m := range reImportNamed.FindAllStringSubmatch(src, -1) {
			if importListHas(m[1], sc.superName) {
				sc.superSpec = m[2]
				break
			}
		}
	}
	sc.properties = parseLitProperties(sc.body)
	if i := strings.Index(sc.body, "constructor("); i >= 0 {
		if open := strings.Index(sc.body[i:], "{"); open >= 0 {
			if end := matchBrace(sc.body, i+open); end > 0 {
				for _, m := range reCtorAssign.FindAllStringSubmatch(sc.body[i+open:end], -1) {
					if _, seen := sc.defaults[m[1]]; !seen {
						sc.defaults[m[1]] = strings.TrimSpace(m[2])
					}
				}
			}
		}
	}
	seenEv := map[string]bool{}
	for _, m := range reNewEvent.FindAllStringSubmatch(sc.body, -1) {
		if !seenEv[m[2]] {
			seenEv[m[2]] = true
			sc.events = append(sc.events, m[2])
		}
	}
	return sc, true
}

// importListHas reports whether an `{ a, b as c }` import/export list binds name.
func importListHas(list, name string) bool {
	for _, part := range strings.Split(list, ",") {
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "type ")))
		if len(fields) == 0 {
			continue
		}
		bound := fields[len(fields)-1] // `a` or `a as c` → c
		if bound == name {
			return true
		}
	}
	return false
}

// parseLitProperties reads `static properties = { … }` from a class body.
// A `...Other.properties` spread is the superclass's set, which the superclass
// walk reads from the superclass itself, so it is skipped here.
func parseLitProperties(body string) []litProperty {
	i := strings.Index(body, "static properties")
	if i < 0 {
		return nil
	}
	open := strings.Index(body[i:], "{")
	if open < 0 {
		return nil
	}
	open += i
	end := matchBrace(body, open)
	if end < 0 {
		return nil
	}
	obj := stripJSComments(body[open+1 : end])
	var out []litProperty
	for _, entry := range splitTopLevel(obj) {
		entry = strings.TrimSpace(entry)
		if entry == "" || strings.HasPrefix(entry, "...") {
			continue
		}
		colon := strings.Index(entry, ":")
		if colon < 0 {
			continue
		}
		key := strings.Trim(strings.TrimSpace(entry[:colon]), `'"`)
		if !reIdent.MatchString(key) {
			continue
		}
		opts := entry[colon+1:]
		if reLitState.MatchString(opts) {
			continue // internal reactive state, not public API
		}
		lp := litProperty{Name: key, Attribute: strings.ToLower(key)}
		if m := reLitType.FindStringSubmatch(opts); m != nil {
			lp.Converter = m[1]
		}
		if reLitAttrOff.MatchString(opts) {
			lp.Attribute = ""
		} else if m := reLitAttrStr.FindStringSubmatch(opts); m != nil {
			lp.Attribute = m[1]
		}
		out = append(out, lp)
	}
	return out
}

// dtsInfo is what a .d.ts says about one class's public fields.
type dtsInfo struct {
	order  []string
	fields map[string]dtsField
}

type dtsField struct{ typ, doc string }

var (
	reDtsField  = regexp.MustCompile(`^\s*((?:(?:public|readonly|declare)\s+)*)([A-Za-z_$][\w$]*)\??\s*:\s*(.+?);\s*$`)
	reDtsGetter = regexp.MustCompile(`^\s*get\s+([A-Za-z_$][\w$]*)\(\)\s*:\s*(.+?);\s*$`)
)

// declarationInfo reads the .d.ts beside a compiled .js for class name.
func (p *elementsPackage) declarationInfo(jsFile, name string) dtsInfo {
	info := dtsInfo{fields: map[string]dtsField{}}
	dtsFile := strings.TrimSuffix(jsFile, ".js") + ".d.ts"
	b, err := os.ReadFile(dtsFile)
	if err != nil {
		return info
	}
	src := string(b)
	loc := regexp.MustCompile(`\bclass\s+` + regexp.QuoteMeta(name) + `\b[^{]*\{`).FindStringIndex(src)
	if loc == nil {
		return info
	}
	end := matchBrace(src, loc[1]-1)
	if end < 0 {
		return info
	}
	var doc []string
	inDoc := false
	depth := 0 // brace depth inside the class body: a method's object-literal return type is not a field
	for _, line := range strings.Split(src[loc[1]:end], "\n") {
		t := strings.TrimSpace(line)
		if !inDoc && !strings.HasPrefix(t, "/**") {
			lineDepth := depth
			depth += strings.Count(t, "{") - strings.Count(t, "}")
			if lineDepth > 0 {
				doc = nil
				continue
			}
		}
		switch {
		case strings.HasPrefix(t, "/**"):
			doc = nil
			inDoc = !strings.HasSuffix(t, "*/")
			doc = append(doc, cleanDocLine(t))
			continue
		case inDoc:
			if strings.HasSuffix(t, "*/") {
				inDoc = false
			}
			doc = append(doc, cleanDocLine(t))
			continue
		}
		var fname, typ string
		if m := reDtsField.FindStringSubmatch(line); m != nil && !strings.Contains(m[2], "(") {
			fname, typ = m[2], m[3]
		} else if m := reDtsGetter.FindStringSubmatch(line); m != nil {
			fname, typ = m[1], m[2]
		}
		text := strings.Join(strings.Fields(strings.Join(doc, " ")), " ")
		doc = nil
		if fname == "" || strings.HasPrefix(t, "protected ") || strings.HasPrefix(t, "private ") ||
			strings.HasPrefix(t, "static ") || strings.HasPrefix(t, "#") {
			continue
		}
		if _, dup := info.fields[fname]; dup {
			continue
		}
		info.order = append(info.order, fname)
		info.fields[fname] = dtsField{typ: p.expandTypeIn(dtsFile, strings.TrimSpace(typ), 0), doc: text}
	}
	return info
}

func cleanDocLine(s string) string {
	s = strings.TrimPrefix(s, "/**")
	s = strings.TrimSuffix(s, "*/")
	s = strings.TrimPrefix(strings.TrimSpace(s), "*")
	return strings.TrimSpace(s)
}

// expandTypes replaces a named type alias (`ButtonSize`) with what it stands
// for (`'sm' | 'md' | 'lg'`) where the declaring module's .d.ts defines it — the
// manifest records the alias NAME, which tells a reader nothing about the
// values an attribute accepts.
func (p *elementsPackage) expandTypes(ref *elementReference) {
	moduleOf := map[string]string{ref.Class: ref.Module}
	for _, m := range p.Manifest.Modules {
		for _, d := range m.Declarations {
			if d.Kind == "class" {
				if _, ok := moduleOf[d.Name]; !ok {
					moduleOf[d.Name] = m.Path
				}
			}
		}
	}
	expand := func(f *elementField) {
		mod, ok := moduleOf[f.DeclaredBy]
		if !ok {
			return // read from source: declarationInfo already expanded it
		}
		if dts := p.compiledFile(mod, ".d.ts"); dts != "" {
			f.Type = p.expandTypeIn(dts, f.Type, 0)
		}
	}
	for i := range ref.Attributes {
		expand(&ref.Attributes[i])
	}
	for i := range ref.Properties {
		expand(&ref.Properties[i])
	}
}

// expandTypeIn expands each bare identifier of a `|` union that file (or a file
// it imports from, relatively) declares as a one-line `export type X = …;`
// alias. Anything else is left exactly as written.
func (p *elementsPackage) expandTypeIn(file, typ string, depth int) string {
	if typ == "" || depth > 3 {
		return typ
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return typ
	}
	src := string(b)
	parts := strings.Split(typ, "|")
	changed := false
	for i, part := range parts {
		id := strings.TrimSpace(part)
		if !reIdent.MatchString(id) || isBuiltinType(id) {
			continue
		}
		if rhs, ok := aliasIn(src, id); ok {
			parts[i] = " " + p.expandTypeIn(file, rhs, depth+1) + " "
			changed = true
			continue
		}
		for _, m := range reImportNamed.FindAllStringSubmatch(src, -1) {
			if importListHas(m[1], id) && isRelativeSpec(m[2]) {
				other := filepath.Join(filepath.Dir(file), filepath.FromSlash(strings.TrimSuffix(m[2], ".js")+".d.ts"))
				if exp := p.expandTypeIn(other, id, depth+1); exp != id {
					parts[i] = " " + exp + " "
					changed = true
				}
				break
			}
		}
	}
	if !changed {
		return typ
	}
	return strings.Join(strings.Fields(strings.Join(parts, "|")), " ")
}

func aliasIn(src, id string) (string, bool) {
	m := regexp.MustCompile(`(?m)^\s*export\s+(?:declare\s+)?type\s+` + regexp.QuoteMeta(id) + `\s*=\s*([^;{}\n]+);`).FindStringSubmatch(src)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

func isBuiltinType(id string) bool {
	switch id {
	case "string", "number", "boolean", "null", "undefined", "unknown", "any", "never", "void", "object":
		return true
	}
	return false
}

// matchBrace returns the index of the `}` closing the `{` at open, skipping
// string literals, template literals (including `${…}` expressions) and
// comments, or -1 when the source is unbalanced.
func matchBrace(s string, open int) int {
	depth := 0
	var tmpl []int // depth at which each open `${` began
	inTemplate := false
	for i := open; i < len(s); i++ {
		c := s[i]
		if inTemplate {
			switch {
			case c == '\\':
				i++
			case c == '`':
				inTemplate = false
			case c == '$' && i+1 < len(s) && s[i+1] == '{':
				tmpl = append(tmpl, depth)
				depth++
				inTemplate = false
				i++
			}
			continue
		}
		switch c {
		case '\'', '"':
			for i++; i < len(s) && s[i] != c; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case '`':
			inTemplate = true
		case '/':
			if i+1 < len(s) && s[i+1] == '/' {
				for i < len(s) && s[i] != '\n' {
					i++
				}
			} else if i+1 < len(s) && s[i+1] == '*' {
				if j := strings.Index(s[i+2:], "*/"); j >= 0 {
					i += j + 3
				} else {
					return -1
				}
			}
		case '{':
			depth++
		case '}':
			depth--
			if n := len(tmpl); n > 0 && depth == tmpl[n-1] {
				tmpl = tmpl[:n-1]
				inTemplate = true
				continue
			}
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// stripJSComments removes // and /* */ comments outside string literals.
func stripJSComments(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' || c == '"':
			j := i + 1
			for j < len(s) && s[j] != c {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			if j >= len(s) {
				j = len(s) - 1
			}
			b.WriteString(s[i : j+1])
			i = j
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			for i < len(s) && s[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return b.String()
			}
			i += j + 3
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// splitTopLevel splits an object-literal body on commas at nesting depth 0.
func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case '\'', '"':
			q := s[i]
			for i++; i < len(s) && s[i] != q; i++ {
				if s[i] == '\\' {
					i++
				}
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}
