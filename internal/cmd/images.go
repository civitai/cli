package cmd

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/civitai/cli/internal/api"
	"github.com/spf13/cobra"
)

const imagesLimitMax = 200

func newImagesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "images",
		Short: "Search images on Civitai",
		Long: `Read-only access to Civitai images via the public REST API
(GET /api/v1/images). Works anonymously.`,
		Example: `  civitai images search --limit 5
  civitai images search --model-id 4384 --period Month
  civitai images search --base-model "Krea 2" --sort "Most Reactions" --period Week
  civitai images search --nsfw --sort "Most Reactions" --period Month --meta
  civitai images get 136456589`,
	}
	cmd.AddCommand(newImagesSearchCmd())
	cmd.AddCommand(newImagesGetCmd())
	return cmd
}

func newImagesSearchCmd() *cobra.Command {
	var (
		username, sort, period, cursor, typ          string
		baseModels                                   []string
		postID, modelID, modelVersionID, limit, page int
		nsfw, meta                                   bool
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search images (GET /api/v1/images)",
		Long: `Search images via GET /api/v1/images.

Pagination: use --page for shallow paging or --cursor for deep paging (the API
caps page*limit at 1000). The next cursor is printed after the results.`,
		Example: `  civitai images search --limit 5
  civitai images search --model-version-id 128713 --sort Newest
  civitai images search --base-model "Krea 2" --sort "Most Reactions" --period Week
  civitai images search --type video --sort "Most Reactions"
  civitai images search --nsfw --sort "Most Reactions" --period Month --meta
  civitai images search --username some-user --cursor <cursor>`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkLimit(limit, imagesLimitMax); err != nil {
				return err
			}
			// The API silently ignores `sort` when `modelId` is set (results come
			// back in the API's default order regardless), so warn rather than
			// let the user believe their --sort took effect. Note: --model-version-id
			// does NOT share this quirk — it honours --sort — so it's excluded here.
			if cmd.Flags().Changed("model-id") && cmd.Flags().Changed("sort") {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"note: the API ignores --sort when --model-id is set; results are returned in the API's default order.")
			}
			o := readFlags(cmd)
			q := url.Values{}
			addIfSet(q, "username", username)
			addIfSet(q, "type", typ) // image | video | audio
			addIfSet(q, "sort", sort)
			addIfSet(q, "period", period)
			addIfSet(q, "cursor", cursor)
			// baseModels is a repeatable filter → repeated `baseModels=` query
			// keys, which the API's zod union (string | string[]) parses as an
			// array (baseModel IN (...), i.e. OR across the given values).
			for _, bm := range baseModels {
				if bm != "" {
					q.Add("baseModels", bm)
				}
			}
			addIfPositive(q, "postId", postID)
			addIfPositive(q, "modelId", modelID)
			addIfPositive(q, "modelVersionId", modelVersionID)
			addIfPositive(q, "limit", limit)
			addIfPositive(q, "page", page)
			if cmd.Flags().Changed("nsfw") {
				q.Set("nsfw", strconv.FormatBool(nsfw))
			}
			// meta is opt-in behind withMeta=true (the API omits it by default to
			// keep payloads small). flatMeta=true forces the flat meta shape on
			// every query route so the parser never has to branch on shape.
			if meta {
				q.Set("withMeta", "true")
				q.Set("flatMeta", "true")
			}

			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			res, err := client.SearchImages(context.Background(), q)
			if err != nil {
				return err
			}
			// --base-model is matched literally (the API OR-matches the given
			// values), so a typo just matches nothing and returns "No images
			// found." at exit 0 with no signal. When --base-model was given and the
			// page came back empty, hint that the spelling may be off. Not an error
			// — zero results is a valid outcome — so this only adds a stderr note.
			if cmd.Flags().Changed("base-model") && len(res.Items) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"note: 0 results — check --base-model spelling (it's matched literally).")
			}
			if o.json {
				return emitJSON(cmd, res.Raw)
			}
			if meta {
				printImageListMeta(cmd, res.Items)
			} else {
				printImageList(cmd, res.Items)
			}
			printPageFooter(cmd, "civitai images search", res.Metadata)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "results per page (1-200)")
	cmd.Flags().IntVar(&page, "page", 0, "page number (shallow paging; prefer --cursor)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "pagination cursor from a previous response")
	cmd.Flags().IntVar(&postID, "post-id", 0, "filter by post id")
	cmd.Flags().IntVar(&modelID, "model-id", 0, "filter by model id")
	cmd.Flags().IntVar(&modelVersionID, "model-version-id", 0, "filter by model version id")
	cmd.Flags().StringVar(&username, "username", "", "filter by uploader username")
	cmd.Flags().StringSliceVar(&baseModels, "base-model", nil, "filter by base model; repeatable (e.g. --base-model \"Krea 2\" --base-model Flux). The API OR-combines the given values")
	cmd.Flags().StringVar(&typ, "type", "", "filter by media type (image, video, audio)")
	cmd.Flags().StringVar(&period, "period", "", "time period (AllTime, Year, Month, Week, Day)")
	cmd.Flags().StringVar(&sort, "sort", "", "sort order (\"Most Reactions\", \"Most Comments\", Newest)")
	cmd.Flags().BoolVar(&nsfw, "nsfw", false, "include NSFW results")
	cmd.Flags().BoolVar(&meta, "meta", false, "include generation metadata (prompt, sampler, seed, etc.)")
	bindReadFlags(cmd)
	return cmd
}

func newImagesGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a single image by id (GET /api/v1/images?imageId=<id>)",
		Long: `Fetch one image by its numeric id — the id in a civitai.com/images/<id>
URL — and render its generation metadata (prompt, settings, resources), the same
detail block as ` + "`images search --meta`" + `. Generation metadata is requested
implicitly.`,
		Example: `  civitai images get 136456589
  civitai images get 136456589 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := readFlags(cmd)
			// A bad positional id is a client-side usage mistake, not an API
			// failure — tag it as a usage error (exit 2), mirroring
			// `model-versions get`. A non-int, non-positive, or out-of-range id
			// (image ids are 32-bit) is caught locally so an oversized id can't
			// leak a host 500 (exit 1) instead of a clean usage error.
			n, err := strconv.Atoi(args[0])
			if err != nil || n <= 0 || n > math.MaxInt32 {
				return asUsageError(fmt.Errorf("image id must be a positive integer, got %q", args[0]))
			}
			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			im, raw, err := client.GetImage(context.Background(), args[0])
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, raw)
			}
			printImageMetaBlock(cmd.OutOrStdout(), *im)
			return nil
		},
	}
	bindReadFlags(cmd)
	return cmd
}

func printImageList(cmd *cobra.Command, items []api.ImageItem) {
	out := cmd.OutOrStdout()
	if len(items) == 0 {
		fmt.Fprintln(out, "No images found.")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tUPLOADER\tBASE MODEL\tSIZE\tNSFW\tHEARTS\tCOMMENTS\tURL")
	for _, im := range items {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%dx%d\t%s\t%d\t%d\t%s\n",
			im.ID, orDash(safeTerm(im.Username)), orDash(truncate(safeTerm(im.BaseModel), 24)),
			im.Width, im.Height, orDash(safeTerm(im.NSFWLevel)),
			im.Stats.HeartCount, im.Stats.CommentCount, safeTerm(im.URL))
	}
	_ = tw.Flush()
}

// printImageListMeta renders each image as an indented detail block instead of
// the compact table, so it can carry the generation metadata (prompt, settings)
// that --meta requests. Every server-origin string is routed through safeTerm —
// prompts are attacker-controlled user text and can carry ANSI/control bytes.
func printImageListMeta(cmd *cobra.Command, items []api.ImageItem) {
	out := cmd.OutOrStdout()
	if len(items) == 0 {
		fmt.Fprintln(out, "No images found.")
		return
	}
	for _, im := range items {
		printImageMetaBlock(out, im)
	}
}

// printImageMetaBlock renders one image as an indented detail block carrying its
// generation metadata (prompt, settings, and the resources "recipe"). Every
// server-origin string is routed through safeTerm — prompts, resource names and
// hashes are attacker-controlled user text that can carry ANSI/control bytes.
// Shared by `images search --meta` and `images get`.
func printImageMetaBlock(out io.Writer, im api.ImageItem) {
	fmt.Fprintf(out, "%d  [%s]  %dx%d  by %s\n",
		im.ID, orDash(safeTerm(im.NSFWLevel)), im.Width, im.Height, orDash(safeTerm(im.Username)))
	m, state := im.ParseMeta()
	switch state {
	case api.MetaAbsent:
		// meta: null — either the uploader hid their generation data, or the
		// API had none for this image. Not an error.
		fmt.Fprintln(out, "  meta: (hidden by uploader)")
	case api.MetaUnparseable:
		// meta present but not the expected object shape — degrade rather than
		// drop the whole image.
		fmt.Fprintln(out, "  meta: (unrecognized format)")
	default: // api.MetaOK
		fmt.Fprintf(out, "  model: %s   sampler: %s   cfg: %s   steps: %s   seed: %s\n",
			orDash(safeTerm(m.Model)), orDash(safeTerm(m.Sampler)),
			orDash(safeTerm(m.CfgScaleString())), orDash(safeTerm(m.StepsString())),
			orDash(safeTerm(m.SeedString())))
		if strings.TrimSpace(m.Prompt) != "" {
			fmt.Fprintf(out, "  prompt: %s\n", safeTerm(m.Prompt))
		}
		if strings.TrimSpace(m.NegativePrompt) != "" {
			fmt.Fprintf(out, "  negative: %s\n", safeTerm(m.NegativePrompt))
		}
		printImageResources(out, m)
	}
	fmt.Fprintf(out, "  url: %s\n", safeTerm(im.URL))
}

// printImageResources renders the meta.resources reproduction recipe — one line
// per resource (checkpoint/LoRA/embedding) with its type, name, weight (when the
// generator supplied one) and hash (inline, else resolved from meta.hashes). The
// section is omitted entirely when there are no resources, so images without a
// recipe stay compact.
func printImageResources(out io.Writer, m api.ImageMeta) {
	rs := m.ParseResources()
	if len(rs) == 0 {
		return
	}
	fmt.Fprintln(out, "  resources:")
	for _, r := range rs {
		line := fmt.Sprintf("    - [%s] %s",
			orDash(safeTerm(strings.TrimSpace(r.Type))), orDash(safeTerm(strings.TrimSpace(r.Name))))
		if w := strings.TrimSpace(r.WeightString()); w != "" {
			line += "  weight " + safeTerm(w)
		}
		if h := strings.TrimSpace(m.ResolveHash(r)); h != "" {
			line += "  hash " + safeTerm(h)
		}
		fmt.Fprintln(out, line)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
