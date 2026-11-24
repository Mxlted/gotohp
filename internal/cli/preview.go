package cli

import (
	"encoding/json"

	"app/core"

	"github.com/spf13/cobra"
)

type previewItem struct {
	Kind  core.UploadWorkKind `json:"kind"`
	Paths []string            `json:"paths"`
}

type uploadPreview struct {
	DryRun   bool            `json:"dryRun"`
	Planned  int             `json:"planned"`
	Items    []previewItem   `json:"items"`
	Skipped  []uploadWarning `json:"skipped"`
	Warnings []uploadWarning `json:"warnings"`
}

func writeUploadPreview(cmd *cobra.Command, paths []string, opts core.UploadOptions) error {
	items, warnings, err := core.PlanUpload(paths, opts, func() bool { return cmd.Context().Err() != nil })
	if err != nil {
		return err
	}
	preview := uploadPreview{
		DryRun:   true,
		Planned:  len(items),
		Items:    make([]previewItem, 0, len(items)),
		Skipped:  []uploadWarning{},
		Warnings: []uploadWarning{},
	}
	for _, item := range items {
		preview.Items = append(preview.Items, previewItem{Kind: item.Kind, Paths: item.Paths()})
	}
	for _, warning := range warnings {
		entry := uploadWarning{Paths: warning.Paths, Code: warning.Code, Message: warning.Message}
		if core.IsSkippedPreflightWarning(warning.Code) {
			preview.Skipped = append(preview.Skipped, entry)
		} else {
			preview.Warnings = append(preview.Warnings, entry)
		}
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(preview)
}
