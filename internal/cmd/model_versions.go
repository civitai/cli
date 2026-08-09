package cmd

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/civitai/cli/pkg/civitai"
	"github.com/spf13/cobra"
)

func newModelVersionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "model-versions",
		Short:   "Inspect model versions on Civitai",
		Aliases: []string{"model-version", "mv"},
		Example: `  civitai model-versions get 128713
  civitai mv get 128713 --json
  civitai model-versions by-hash 5D8D26E2A6`,
	}
	// The alias list is rendered FROM cmd.Aliases rather than retyped, so a
	// dropped or added alias cannot leave the help advertising a spelling the
	// tree no longer answers to. TestReadAPIHelpNamesItsAliases pins it.
	cmd.Long = `Read-only access to Civitai model versions through the public REST API
(GET /api/v1/model-versions/{id} and .../by-hash/{hash}).
Aliases: ` + strings.Join(cmd.Aliases, ", ") + `.

` + readAnonNote + `

A model VERSION is the downloadable unit, and it is what most of the rest of
this CLI wants: ` + "`civitai download --version <id>`" + `,
` + "`civitai generate --checkpoint <id>`" + ` and ` + "`--lora <id>`" + ` all take a version
id, never a model id. ` + "`civitai models get <id>`" + ` is where you read those
version ids off a model.

` + "`by-hash`" + ` runs that lookup backwards: it identifies a file you already have
on disk, which is how you put a name to an unlabelled .safetensors.

` + readJSONNote
	cmd.AddCommand(newModelVersionsGetCmd())
	cmd.AddCommand(newModelVersionsByHashCmd())
	return cmd
}

func newModelVersionsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a model version by id (GET /api/v1/model-versions/{id})",
		Long: `Get one model version by its version id: GET /api/v1/model-versions/{id}.

This is the id ` + "`civitai download --version`" + ` and
` + "`civitai generate --checkpoint / --lora`" + ` take. A non-integer argument is
refused locally, as a usage mistake, before any request is made.

The output carries the base model, the trigger words, the AIR identifier and
every file with its size and type. A version whose primary file is not model
weights is tagged with that type ([Archive], [Training Data], [Other]).

What a version does NOT carry is model-level data: .model is only
{name, type, nsfw, poi} — no creator, no model download count. If you started
from a version and need those, read them from ` + "`models get`" + ` /
` + "`models search`" + ` and join on the version's .modelId.

` + readAnonShort,
		Example: `  civitai model-versions get 128713
  civitai mv get 128713 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := readFlags(cmd)
			if _, err := strconv.Atoi(args[0]); err != nil {
				// A non-integer positional arg is a client-side usage mistake, not
				// an API failure — tag it so the entrypoint maps it to the usage
				// exit code rather than the generic one.
				return asUsageError(fmt.Errorf("model version id must be an integer, got %q", args[0]))
			}
			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			v, raw, err := client.GetModelVersion(context.Background(), args[0])
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, raw)
			}
			printModelVersionDetail(cmd, v)
			return nil
		},
	}
	bindReadFlags(cmd)
	return cmd
}

func newModelVersionsByHashCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "by-hash <hash>",
		Short: "Get a model version by file hash (GET /api/v1/model-versions/by-hash/{hash})",
		Long: `Look up a model version by any of its file hashes:
GET /api/v1/model-versions/by-hash/{hash}.

AutoV1, AutoV2, SHA256, CRC32 and BLAKE3 hashes all work — the server
upper-cases the value, so case does not matter here. This is the "what IS this
file?" lookup for a .safetensors you already have on disk.

Note the API reports SHA256 in UPPER case while sha256sum prints lower case, so
case-fold before comparing if you hash a file yourself. (` + "`civitai download`" + `'s
own verification is already case-insensitive.)

On a hit the CLI prints a ready-to-run download line for the resolved version.
It uses --version rather than a bare positional id deliberately: a bare id is
ambiguous between a model id and a version id, and the printed command must
never be the one that trips that stop.

` + readAnonShort,
		Example: `  civitai model-versions by-hash 5D8D26E2A6
  civitai mv by-hash 5D8D26E2A6 --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := readFlags(cmd)
			client, _, err := newReader(o)
			if err != nil {
				return err
			}
			v, raw, err := client.GetModelVersionByHash(context.Background(), args[0])
			if err != nil {
				return err
			}
			if o.json {
				return emitJSON(cmd, raw)
			}
			printModelVersionDetail(cmd, v)
			// A by-hash lookup is almost always the prelude to a download, so emit a
			// ready-to-run command. It uses --version (never a bare positional id) so
			// the printed command can never trip the download model-id/version-id
			// ambiguity stop, and pairs it with --layout for type-routed placement.
			printDownloadCommandHint(cmd.OutOrStdout(), v)
			return nil
		},
	}
	bindReadFlags(cmd)
	return cmd
}

// printDownloadCommandHint prints a runnable, ambiguity-gate-immune download
// command for a resolved version (empty/zero version → nothing). Using --version
// keeps the suggested command from ever hitting the model-id/version-id ambiguity
// stop; --layout comfyui is a sensible type-routed default the user can drop or
// change (e.g. --layout a1111, or --out-dir).
func printDownloadCommandHint(out io.Writer, v *civitai.ModelVersionDetail) {
	if v == nil || v.ID == 0 {
		return
	}
	fmt.Fprintf(out, "  ↳ download: civitai download --version %d --layout comfyui\n", v.ID)
}

func printModelVersionDetail(cmd *cobra.Command, v *civitai.ModelVersionDetail) {
	out := cmd.OutOrStdout()
	header := fmt.Sprintf("%s (version id %d, model id %d)", safeTerm(v.Name), v.ID, v.ModelID)
	if mk := nonModelFileMarker(v.Files); mk != "" {
		header += " " + mk
	}
	fmt.Fprintln(out, header)
	if v.Model != nil {
		fmt.Fprintf(out, "  model:     %s (%s)\n", safeTerm(v.Model.Name), safeTerm(v.Model.Type))
	}
	fmt.Fprintf(out, "  baseModel: %s\n", safeTerm(v.BaseModel))
	if v.AIR != "" {
		fmt.Fprintf(out, "  air:       %s\n", safeTerm(v.AIR))
	}
	fmt.Fprintf(out, "  downloads: %d   thumbsUp: %d\n", v.Stats.DownloadCount, v.Stats.ThumbsUpCount)
	if len(v.TrainedWords) > 0 {
		fmt.Fprintf(out, "  triggers:  %s\n", joinTags(v.TrainedWords))
	}
	if v.DownloadURL != "" {
		fmt.Fprintf(out, "  download:  %s\n", safeTerm(v.DownloadURL))
	}
	if len(v.Files) > 0 {
		fmt.Fprintf(out, "  files (%d):\n", len(v.Files))
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		for _, f := range v.Files {
			fmt.Fprintf(tw, "    %s\t%s\t%.1f MB\n", safeTerm(f.Name), safeTerm(f.Type), f.SizeKB/1024)
		}
		_ = tw.Flush()
	}
}
