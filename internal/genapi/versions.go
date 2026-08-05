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
//     default, and that correction is surfaced on-site but is invisible through
//     a non-browser path.
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
