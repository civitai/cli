package cmd

import (
	"context"
	"fmt"
	"strconv"
	"text/tabwriter"

	"github.com/civitai/cli/internal/api"
	"github.com/spf13/cobra"
)

func newModelVersionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "model-versions",
		Short:   "Inspect model versions on Civitai",
		Aliases: []string{"model-version", "mv"},
		Long: `Read-only access to Civitai model versions via the public REST API. Look up a
version by its id or by a file hash (AutoV2, SHA256, …). Works anonymously.`,
		Example: `  civitai model-versions get 128713
  civitai model-versions by-hash 5D8D26E2A6`,
	}
	cmd.AddCommand(newModelVersionsGetCmd())
	cmd.AddCommand(newModelVersionsByHashCmd())
	return cmd
}

func newModelVersionsGetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <id>",
		Short:   "Get a model version by id (GET /api/v1/model-versions/{id})",
		Example: `  civitai model-versions get 128713`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o := readFlags(cmd)
			if _, err := strconv.Atoi(args[0]); err != nil {
				return fmt.Errorf("model version id must be an integer, got %q", args[0])
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
		Use:     "by-hash <hash>",
		Short:   "Get a model version by file hash (GET /api/v1/model-versions/by-hash/{hash})",
		Long:    `Look up a model version by any of its file hashes (AutoV1, AutoV2, SHA256, CRC32, BLAKE3). The hash is matched case-insensitively.`,
		Example: `  civitai model-versions by-hash 5D8D26E2A6`,
		Args:    cobra.ExactArgs(1),
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
			return nil
		},
	}
	bindReadFlags(cmd)
	return cmd
}

func printModelVersionDetail(cmd *cobra.Command, v *api.ModelVersionDetail) {
	out := cmd.OutOrStdout()
	header := fmt.Sprintf("%s (version id %d, model id %d)", v.Name, v.ID, v.ModelID)
	if mk := nonModelFileMarker(v.Files); mk != "" {
		header += " " + mk
	}
	fmt.Fprintln(out, header)
	if v.Model != nil {
		fmt.Fprintf(out, "  model:     %s (%s)\n", v.Model.Name, v.Model.Type)
	}
	fmt.Fprintf(out, "  baseModel: %s\n", v.BaseModel)
	if v.AIR != "" {
		fmt.Fprintf(out, "  air:       %s\n", v.AIR)
	}
	fmt.Fprintf(out, "  downloads: %d   thumbsUp: %d\n", v.Stats.DownloadCount, v.Stats.ThumbsUpCount)
	if len(v.TrainedWords) > 0 {
		fmt.Fprintf(out, "  triggers:  %s\n", joinTags(v.TrainedWords))
	}
	if v.DownloadURL != "" {
		fmt.Fprintf(out, "  download:  %s\n", v.DownloadURL)
	}
	if len(v.Files) > 0 {
		fmt.Fprintf(out, "  files (%d):\n", len(v.Files))
		tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
		for _, f := range v.Files {
			fmt.Fprintf(tw, "    %s\t%s\t%.1f MB\n", f.Name, f.Type, f.SizeKB/1024)
		}
		_ = tw.Flush()
	}
}
