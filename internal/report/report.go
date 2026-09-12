package report

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/medom/terraform-drift-detector/internal/model"
)

func Build(statePath string, expected, actual []model.Resource, drifts []model.Drift, warnings []string) model.Report {
	sum := model.Summary{
		Expected:    len(expected),
		Actual:      len(actual),
		TotalDrifts: len(drifts),
	}
	for _, d := range drifts {
		switch d.Kind {
		case model.KindDeleted:
			sum.Deleted++
		case model.KindCreated:
			sum.Created++
		case model.KindModified:
			sum.Modified++
		case model.KindTagChanged:
			sum.TagChanged++
		}
	}
	if drifts == nil {
		drifts = []model.Drift{}
	}
	return model.Report{
		ScannedAt: time.Now().UTC(),
		StatePath: statePath,
		Summary:   sum,
		Drifts:    drifts,
		Warnings:  warnings,
	}
}

func WriteJSON(w io.Writer, r model.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func WriteTable(w io.Writer, r model.Report) error {
	fmt.Fprintf(w, "Scanned: %s\nState:   %s\n", r.ScannedAt.Format(time.RFC3339), r.StatePath)
	fmt.Fprintf(w, "Summary: %d expected, %d live, %d drifts (deleted=%d modified=%d tags=%d created=%d)\n\n",
		r.Summary.Expected, r.Summary.Actual, r.Summary.TotalDrifts,
		r.Summary.Deleted, r.Summary.Modified, r.Summary.TagChanged, r.Summary.Created)
	if len(r.Warnings) > 0 {
		fmt.Fprintln(w, "Warnings:")
		for _, warn := range r.Warnings {
			fmt.Fprintf(w, "  - %s\n", warn)
		}
		fmt.Fprintln(w)
	}
	if len(r.Drifts) == 0 {
		fmt.Fprintln(w, "No drift detected.")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tADDRESS\tPATH\tBEFORE\tAFTER")
	for _, d := range r.Drifts {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			d.Kind, d.Address, d.Path, compact(d.Before), compact(d.After))
	}
	return tw.Flush()
}

func compact(v any) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprint(v)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 60 {
		return s[:57] + "..."
	}
	return s
}

func SaveFile(path string, r model.Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return WriteJSON(f, r)
}

func LoadFile(path string) (model.Report, error) {
	var r model.Report
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}
