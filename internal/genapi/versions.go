package genapi

import (
	"context"
	"fmt"
	"strconv"

	"github.com/civitai/cli/pkg/civitai"
)

// ResolvedVersion is what a model-version id resolves to: the model TYPE the
// graph requires, plus the names a confirmation prompt should show.
type ResolvedVersion struct {
	// VersionID is the id that was resolved.
	VersionID int
	// VersionName is the version's own name, e.g. "v8".
	VersionName string
	// ModelName is the parent model's name, e.g. "DreamShaper".
	ModelName string
	// ModelType is the graph-required type, e.g. "Checkpoint", "LORA".
	ModelType string
	// BaseModel is the version's base model, e.g. "SDXL 1.0". Informational.
	BaseModel string
}

// DisplayName renders the resolved version for a human confirmation, so the
// user approves a NAME rather than an integer.
func (r *ResolvedVersion) DisplayName() string {
	switch {
	case r.ModelName != "" && r.VersionName != "":
		return r.ModelName + " — " + r.VersionName
	case r.ModelName != "":
		return r.ModelName
	case r.VersionName != "":
		return r.VersionName
	default:
		return strconv.Itoa(r.VersionID)
	}
}

// Resource builds the graph resource reference for this version. The model
// type is carried because Graph.Resources entries REQUIRE `model:{type}` — a
// bare id, or `{id}` alone, is rejected with a 400.
func (r *ResolvedVersion) Resource(strength *float64) Resource {
	return Resource{
		ID:       r.VersionID,
		Model:    &ResourceModel{Type: r.ModelType},
		Strength: strength,
	}
}

// ResolveModelVersion resolves a model-version id via the existing PUBLIC read
// route GET /api/v1/model-versions/{id}.
//
// It exists for two reasons, both of which are about money:
//
//  1. Graph.Resources entries REQUIRE the model type, which is not derivable
//     from a version id, and a WRONG type is silently accepted (a LoRA sent as
//     {type:"Checkpoint"} returns 200 with the cost unchanged). So the type has
//     to come from a live lookup, never a guess.
//  2. It converts a nonexistent checkpoint id from a SILENT SUBSTITUTED CHARGE
//     into a hard local 404. Measured: on one ecosystem the correct model id
//     priced at 160, while a nonexistent id, a foreign-ecosystem id, and no
//     model at all ALL priced at 60 — the server substitutes the ecosystem
//     default and bills for it.
//
// 🔴 THE SUBSTITUTION IS NO LONGER INVISIBLE — an earlier revision of this
// comment said the correction "is surfaced on-site but is invisible through a
// non-browser path", and that has been FALSE since civitai#3665 (PRs #3692 /
// #3673). The server now reports every swap as `modelSubstitutions`; see
// substitution.go for the three read sites. That does NOT make this lookup
// redundant, and the reasons are worth keeping straight:
//
//   - The report is post-hoc on the submit and only advisory on the whatIf,
//     while this turns a bad id into a LOCAL error before any spend at all.
//   - `ModelType` still has to come from somewhere, and no reply carries it.
//   - Absence of the field is ambiguous — "no substitution" and "a server
//     predating the field" are indistinguishable on the wire.
//
// The two are complements: this stops a NONEXISTENT id, and the server's signal
// catches the case it structurally cannot — a REAL id that is wrong for the
// chosen ecosystem or workflow.
//
// This is a live round-trip, NOT a vendored table, so it carries no drift cost
// against the platform — the distinction that makes it the right fix under this
// repo's anti-mirror rule.
//
// Errors carry the read SDK's classification (a missing version is tagged
// civitai.ErrNotFound), so exit codes stay pinned by errors.Is.
func (c *Client) ResolveModelVersion(ctx context.Context, id int) (*ResolvedVersion, error) {
	if id <= 0 {
		return nil, civitai.Tag(civitai.ErrBadRequest,
			fmt.Errorf("invalid model-version id %d — pass the numeric id from a model's version URL (…/models/<model-id>?modelVersionId=<version-id>)", id))
	}
	getter := c.Versions
	if getter == nil {
		getter = civitai.NewWithSource(c.BaseURL, c.Tokens)
	}
	v, _, err := getter.GetModelVersion(ctx, strconv.Itoa(id))
	if err != nil {
		return nil, err
	}
	if v == nil || v.Model == nil || v.Model.Type == "" {
		// Without a type the resource cannot be built at all, and guessing one
		// would be silently accepted by the server. Fail loudly instead.
		return nil, fmt.Errorf("model version %d has no model type in the API response — cannot build a generation resource for it", id)
	}
	return &ResolvedVersion{
		VersionID:   id,
		VersionName: v.Name,
		ModelName:   v.Model.Name,
		ModelType:   v.Model.Type,
		BaseModel:   v.BaseModel,
	}, nil
}
