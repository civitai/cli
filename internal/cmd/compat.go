package cmd

import (
	"fmt"
	"strings"
)

// ── base-model compatibility (`download --for-base`) ─────────────────────────
//
// Ancillary resources silently mismatch the target model: an SD 1.5 embedding
// (e.g. EasyNegative) does nothing — or breaks — on an SDXL checkpoint, and the
// wrong VAE yields black images. The CLI already knows each version's BaseModel,
// so `--for-base <baseModel>` warns when the resolved version's base model is in
// a CONFIDENTLY different family than the target.
//
// The classifier groups the common Civitai base models into architecture
// families. It is deliberately CONSERVATIVE: the many SDXL-derived groups (base
// SDXL, Pony, Illustrious, NoobAI, Turbo/Lightning/LCM variants) all collapse to
// ONE "sdxl" family, so near-neighbours (Pony vs Illustrious) never trigger a
// false alarm — only architecture-level mismatches (SD 1.5 vs any SDXL, Flux vs
// SD, SD3 vs SD1, video vs image, …) warn.
//
// Families:
//   sd1    — SD 1.x (SD 1.4, SD 1.5 and its LCM/Hyper/Turbo variants)
//   sd2    — SD 2.x (SD 2.0/2.1, 768 and unCLIP variants)
//   sdxl   — SDXL architecture: SDXL 1.0/0.9/Turbo/Lightning/LCM/Hyper, Pony,
//            Illustrious, NoobAI (loose — distinct compat groups, same arch)
//   sd3    — SD 3.x (SD 3, 3.5 Large/Medium)
//   flux   — Flux.1 (S/D/Kontext)
//   cascade— Stable Cascade
//   pixart — PixArt (α/Σ)
//   kolors — Kolors
//   hunyuan— Hunyuan DiT (image)
//   lumina — Lumina
//   aura   — AuraFlow
//   video  — video base models (Wan, Hunyuan Video, LTXV, Mochi, CogVideoX, SVD)
//   ""     — unknown / unclassified (never warns)

// baseModelFamily returns the architecture family key for a Civitai base-model
// label, or "" when it can't be classified confidently. Matching is
// case-insensitive and prefix/substring based to tolerate the many labelled
// variants the API emits.
func baseModelFamily(baseModel string) string {
	s := strings.ToLower(strings.TrimSpace(baseModel))
	if s == "" {
		return ""
	}
	// Video first — several share "video"/architecture keywords that would
	// otherwise be mistaken for image bases.
	for _, k := range []string{"wan video", "wan ", "hunyuan video", "ltxv", "ltx video", "mochi", "cogvideo", "svd", "stable video"} {
		if strings.Contains(s, k) {
			return "video"
		}
	}
	switch {
	case strings.HasPrefix(s, "flux"):
		return "flux"
	case strings.HasPrefix(s, "sd 3") || strings.HasPrefix(s, "sd3") || strings.HasPrefix(s, "stable diffusion 3"):
		return "sd3"
	case strings.Contains(s, "cascade"):
		return "cascade"
	case strings.HasPrefix(s, "pixart"):
		return "pixart"
	case strings.HasPrefix(s, "kolors"):
		return "kolors"
	case strings.HasPrefix(s, "hunyuan"): // Hunyuan DiT (video handled above)
		return "hunyuan"
	case strings.HasPrefix(s, "lumina"):
		return "lumina"
	case strings.HasPrefix(s, "aura"):
		return "aura"
	}
	// SDXL architecture family (loose — includes the derived checkpoints).
	for _, k := range []string{"sdxl", "pony", "illustrious", "noobai", "noob ai"} {
		if strings.Contains(s, k) {
			return "sdxl"
		}
	}
	// SD 2.x before SD 1.x so "sd 2" isn't shadowed by a bare "sd" test.
	if strings.HasPrefix(s, "sd 2") || strings.HasPrefix(s, "sd2") || strings.HasPrefix(s, "stable diffusion 2") {
		return "sd2"
	}
	if strings.HasPrefix(s, "sd 1") || strings.HasPrefix(s, "sd1") || strings.HasPrefix(s, "stable diffusion 1") {
		return "sd1"
	}
	return ""
}

// baseModelMismatch reports whether have (the resolved version's base model) is
// in a confidently different family than want (--for-base). It returns false
// when either side is unclassifiable (conservative — no false alarms on unknown
// or near-neighbour bases).
func baseModelMismatch(have, want string) bool {
	hf, wf := baseModelFamily(have), baseModelFamily(want)
	if hf == "" || wf == "" {
		return false
	}
	return hf != wf
}

// baseModelWarning returns the stderr warning text for a confident mismatch, or
// "" when the bases are compatible / unclassifiable.
func baseModelWarning(have, want string) string {
	if !baseModelMismatch(have, want) {
		return ""
	}
	return fmt.Sprintf("base model %q may be incompatible with %q (different model families) — this resource likely won't work with your target model", have, want)
}
