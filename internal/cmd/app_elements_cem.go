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
// publishes and builds one element's reference from it ALONE. `app_elements.go`
// is the command that prints it.
//
// 🔴 MEMBERS COME FROM THE ELEMENT'S OWN MANIFEST ENTRY; THE SUPERCLASS WALK
// ONLY NAMES THE CHAIN. The analyzer flattens every inherited member onto the
// element that inherits it, carrying `inheritedFrom`, for each superclass it
// analyzed. What differs by version is which superclasses it analyzed:
//
//   - 0.10.0 and earlier: the element modules only. A shared base that is not
//     itself an element (`CivitaiField`, `CivitaiMediaElement`,
//     `CivitaiElement`) is not in the manifest, so its members are not on the
//     element: `<civitai-text-input>` lists five attributes and none of
//     `label`, `name`, `value`, `description`, `error`, `required`,
//     `disabled`, `size`.
//   - 0.10.1 and later: the base modules too, so those members are on the
//     element with `inheritedFrom`, and every `superclass` resolves to a
//     manifest declaration or to a `package`-qualified root.
//
// So the walk follows `superclass` by module path only to fill `extends` (the
// chain in the header) and `unresolved` (a superclass the manifest does not
// declare, which the command reports as "Incomplete … upgrade
// @civitai/components"). It adds no members: measured over all 47 elements of
// 0.10.0 and 0.10.1, merging the ancestors' members added none the flattened
// entries did not already carry. Nothing compensates for an old manifest — no
// parsing of the package's compiled output, no second source of truth.

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
			// A copy without a manifest is skipped and the walk continues
			// upward; if no copy anywhere has one, the caller reports the
			// manifest as not found, without distinguishing this case.
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

// findDecl finds a class declaration in the manifest by its module path and
// class name. A reference whose module is not in the manifest does not resolve.
func (p *elementsPackage) findDecl(ref cemRef) (elementDecl, bool) {
	want := moduleKey(ref.Module)
	for _, m := range p.Manifest.Modules {
		if ref.Module == "" || moduleKey(m.Path) != want {
			continue
		}
		for _, d := range m.Declarations {
			if d.Kind == "class" && d.Name == ref.Name {
				return elementDecl{Module: m.Path, Decl: d}, true
			}
		}
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

// resolveElement builds one element's reference: every member its manifest
// entry lists (own and flattened-inherited), plus the superclass chain.
func (p *elementsPackage) resolveElement(e elementDecl) elementReference {
	d := e.Decl
	r := elementReference{
		Tag: d.TagName, Class: d.Name, Module: e.Module,
		Description: firstNonEmpty(d.Summary, d.Description),
		Extends:     []string{}, Attributes: []elementField{}, Properties: []elementField{},
		Events: []elementNamed{}, Slots: []elementNamed{}, CSSParts: []elementNamed{},
		CSSProperties: []elementNamed{}, Unresolved: []string{},
	}
	for _, a := range d.Attributes {
		r.Attributes = append(r.Attributes, elementField{
			Name: a.Name, Type: typeText(a.Type), Default: a.Default,
			Description: strings.TrimSpace(a.Description), DeclaredBy: declaredBy(a.InheritedFrom, d.Name),
		})
	}
	for _, m := range d.Members {
		// A field with an attribute is documented by that attribute; the
		// property list holds only the ones with none.
		if m.Kind != "field" || m.Static || m.Attribute != "" {
			continue
		}
		if m.Privacy == "private" || m.Privacy == "protected" || strings.HasPrefix(m.Name, "#") {
			continue
		}
		r.Properties = append(r.Properties, elementField{
			Name: m.Name, Type: typeText(m.Type), Default: m.Default,
			Description: strings.TrimSpace(m.Description), DeclaredBy: declaredBy(m.InheritedFrom, d.Name),
		})
	}
	for _, ev := range d.Events {
		r.Events = append(r.Events, elementNamed{Name: ev.Name, Type: typeText(ev.Type),
			Description: strings.TrimSpace(ev.Description), DeclaredBy: declaredBy(ev.InheritedFrom, d.Name)})
	}
	for _, s := range d.Slots {
		r.Slots = append(r.Slots, elementNamed{Name: s.Name,
			Description: strings.TrimSpace(s.Description), DeclaredBy: declaredBy(s.InheritedFrom, d.Name)})
	}
	for _, s := range d.CSSParts {
		r.CSSParts = append(r.CSSParts, elementNamed{Name: s.Name,
			Description: strings.TrimSpace(s.Description), DeclaredBy: declaredBy(s.InheritedFrom, d.Name)})
	}
	for _, c := range d.CSSProperties {
		r.CSSProperties = append(r.CSSProperties, elementNamed{Name: c.Name, Type: c.Syntax, Default: c.Default,
			Description: strings.TrimSpace(c.Description), DeclaredBy: d.Name})
	}
	r.Extends, r.Unresolved = p.superclassChain(d.Superclass)
	return r
}

// superclassChain follows `superclass` through the manifest's declarations,
// nearest first. It stops at a root (complete) or at a reference the manifest
// does not declare (returned in unresolved). It reads no members.
func (p *elementsPackage) superclassChain(ref *cemRef) (chain, unresolved []string) {
	chain, unresolved = []string{}, []string{}
	visited := map[string]bool{}
	for ref != nil && ref.Name != "" {
		key := ref.Name + "\x00" + moduleKey(ref.Module)
		if visited[key] {
			break
		}
		visited[key] = true
		chain = append(chain, ref.Name)
		if isRootSuperclass(*ref) {
			break
		}
		d, ok := p.findDecl(*ref)
		if !ok {
			unresolved = append(unresolved, ref.Name)
			break
		}
		ref = d.Decl.Superclass
	}
	return chain, unresolved
}

// isRootSuperclass reports whether ref ends the chain WITHOUT anything missing:
// a `package`-qualified class — another package's (`lit`'s LitElement) or a
// platform global (`{name: "HTMLElement", package: "global:"}`). Neither is
// described by this package's manifest, and neither has members an author sets
// on a `<civitai-*>` tag, so reaching one is a complete answer.
func isRootSuperclass(ref cemRef) bool {
	return ref.Package != ""
}
