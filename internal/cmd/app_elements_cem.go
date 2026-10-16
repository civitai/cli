package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// This file reads the Custom Elements Manifest (CEM) that `@civitai/components`
// publishes and resolves one element's FULL member set — its own plus everything
// it inherits — from that manifest ALONE. `app_elements.go` is the command that
// prints it.
//
// 🔴 THE MANIFEST IS THE ONLY SOURCE, AND AN OLD ONE IS REPORTED, NOT PATCHED.
// Up to and including 0.10.0 the analyzer was pointed at the element files only,
// so an element whose superclass is a shared base that is not itself an element
// (`CivitaiField`, `CivitaiMediaElement`, `CivitaiElement`) arrived with only
// its own members and a `superclass` reference to a declaration the manifest
// does not contain: `<civitai-text-input>` listed five attributes and none of
// `label`, `name`, `value`, `description`, `error`, `required`, `disabled`,
// `size`. The fix is upstream — the base modules are analyzed too, so their
// members arrive flattened with `inheritedFrom` and the bases are declarations
// in the manifest. This reader does not compensate for an old manifest by
// parsing the package's compiled output: a superclass it cannot find in the
// manifest is listed in `unresolved`, and the command says the reference is
// incomplete and to upgrade the package. A second, heuristic source of truth
// would be a parser to maintain against every future build layout.

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

// findDecl finds a class declaration in the manifest by reference: by module
// path first, then — when that module is not in the manifest (a re-export
// barrel such as `internals.js`) or no module is given — by class name, if
// exactly one class in the manifest has it.
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
	if len(byName) == 1 {
		return byName[0], true
	}
	return elementDecl{}, false
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
	// Extends is the superclass chain, nearest first, as far as the manifest
	// declares it.
	Extends       []string       `json:"extends"`
	Attributes    []elementField `json:"attributes"`
	Properties    []elementField `json:"properties"`
	Events        []elementNamed `json:"events"`
	Slots         []elementNamed `json:"slots"`
	CSSParts      []elementNamed `json:"cssParts"`
	CSSProperties []elementNamed `json:"cssProperties"`
	// Unresolved names each superclass the manifest does not declare, so an
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

// resolveElement builds the full reference for one element: its own members
// (including any the analyzer already flattened, carrying `inheritedFrom`), then
// each ancestor's, following `superclass` through the manifest's declarations.
// The nearest declaration of a name wins.
//
// followSuperclasses=false is the seam the inheritance test uses to prove the
// inherited assertions depend on this walk; production always passes true.
func (p *elementsPackage) resolveElement(e elementDecl, followSuperclasses bool) elementReference {
	b := newReferenceBuilder(e)
	b.addManifestDecl(e.Decl)
	if followSuperclasses {
		p.walkSuperclasses(b, e.Decl.Superclass)
	}
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
		if isRootSuperclass(*ref) {
			return
		}
		d, ok := p.findDecl(*ref)
		if !ok {
			b.ref.Unresolved = append(b.ref.Unresolved, ref.Name)
			return
		}
		b.addManifestDecl(d.Decl)
		ref = d.Decl.Superclass
	}
}

// isRootSuperclass reports whether ref ends the chain WITHOUT anything missing:
// a class from another package (`lit`'s LitElement) or a platform global
// (`{name: "HTMLElement", package: "global:"}`). Neither is described by this
// package's manifest, and neither has members an author sets on a
// `<civitai-*>` tag, so reaching one is a complete answer — not "Incomplete".
// The two names are also accepted bare, for a manifest that records them
// without a `package`.
func isRootSuperclass(ref cemRef) bool {
	if ref.Package != "" {
		return true
	}
	switch ref.Name {
	case "HTMLElement", "LitElement":
		return true
	}
	return false
}
