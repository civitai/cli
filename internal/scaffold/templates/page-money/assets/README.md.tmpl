# `assets/` — your store-listing media

Nothing in here is served by your app. This directory holds the images your
**store listing** needs, which you upload with `civitai app listing`:

```bash
civitai app listing set-icon       ./assets/icon.png
civitai app listing set-cover      ./assets/cover.png
civitai app listing add-screenshot ./assets/screenshot-1.png --caption "Grid view"
civitai app listing status         # what is attached vs. what publishing still needs
```

An **icon and a cover are mandatory** before a listing can publish. Screenshots
are optional (up to 8). You can attach all of it **while your app is in review**
— it carries forward when a moderator approves the app.

Listing media is uploaded by those commands, separately from your code. This
directory is **not** excluded from `civitai app submit`, though, so whatever you
leave here also rides along in the source bundle — measured at 37 files with an
icon and a cover present. That is harmless at these sizes; just don't park a
folder of rejects in here.

> **This directory ships with no images, on purpose.** A placeholder icon passes
> every format and size check and uploads cleanly, which is how a stub graphic
> ends up on a public store listing. A *missing* file fails loudly, at the one
> moment you can still fix it.

## What to put here

| file | kind | aspect (w ÷ h) | minimum | source file |
| --- | --- | --- | --- | --- |
| `icon.png` | icon (1, required) | 0.9 – 1.1 (square-ish) | 128 px on the shorter side | png / jpeg / webp, ≤ 2 MiB |
| `cover.png` | cover (1, required) | 1.3 – 2.4 (~4:3 → ~21:9) | 640 px wide | png / jpeg / webp, ≤ 4 MiB |
| `screenshot-1.png` … | screenshot (≤ 8, optional) | 0.4 – 2.6 (either orientation) | 320 px on the shorter side | png / jpeg / webp, ≤ 2 MiB |

Easy starting points: a **512 × 512** icon and a **1600 × 900** cover.

## Who checks what

The CLI checks the **format and the byte size** of the file you hand it, before
anything is uploaded — and for an **icon** that is not the whole story, because
the platform measures an image the CLI never sees (first bullet below). The CLI
does **not** check dimensions or aspect ratio — those are validated by the
platform when the image is attached, and the platform is the authority. It does
print what it decoded, so the dimensions every server-side bound depends on are
on screen before the upload (`Uploading icon (37.3 KiB, 1024×1024)…`). A
rejection names the bound and your value, e.g.:

```text
icon must be square-ish (aspect 2.00 outside 0.9–1.1)
cover must be at least 640px wide (got 512px)
```

Two behaviours worth knowing:

- **Icons are re-encoded server-side** to PNG and downscaled to at most 1024 px
  on the longer side (aspect preserved, never enlarged), and the platform caps
  that *re-encoded* PNG at 1 MiB — a different measurement from the 2 MiB the CLI
  applies to your file. A **small** icon stays small, so the 128 px floor is
  real; a **large** one is not automatically fine either. Measured 2026-08-10: a
  1024 × 1024 photographic JPEG of 37.3 KiB was refused because the PNG the
  platform made from it came to ~1.15 MiB, while a 512 × 512 icon in the same run
  went through. The bytes in that rejection are the platform's, not your file's —
  the lever is **pixel dimensions, not heavier compression**. Flat artwork
  re-encodes far smaller.
- **Covers and screenshots are not rescaled.** What you upload is what the store
  shows, so ship them at the size you want rendered.

The numbers above are current platform guidance, not a contract this file can
enforce. If one of them ever moves, the message you get back from an attach is
the truth.
