package main

import (
	"context"
	"fmt"
	"os"

	httpserver "github.com/medom/terraform-drift-detector/internal/http"
	"github.com/medom/terraform-drift-detector/internal/provider"
	awsp "github.com/medom/terraform-drift-detector/internal/provider/aws"
	"github.com/medom/terraform-drift-detector/internal/report"
	"github.com/medom/terraform-drift-detector/internal/scan"
	"github.com/medom/terraform-drift-detector/internal/schedule"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var (
		statePath string
		backendS3 string
		outPath   string
		region    string
		jsonOut   bool
		unmanaged bool
		cronSpec  string
		listen    string
	)

	applyFlags := func(cmd *cobra.Command) {
		cmd.Flags().StringVar(&statePath, "state", "", "path to a local terraform.tfstate file")
		cmd.Flags().StringVar(&backendS3, "backend-s3", "", "S3 state location as bucket/key")
		cmd.Flags().StringVar(&outPath, "out", ".tdd/last-report.json", "path to write the latest JSON report")
		cmd.Flags().StringVar(&region, "region", os.Getenv("AWS_REGION"), "AWS region")
		cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON instead of a table")
		cmd.Flags().BoolVar(&unmanaged, "unmanaged", false, "report live resources not present in state (created)")
	}

	scanCmd := &cobra.Command{
		Use:   "scan",
		Short: "Compare Terraform state to live cloud resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			opt := scan.Options{
				StatePath: statePath,
				BackendS3: backendS3,
				OutPath:   outPath,
				Unmanaged: unmanaged,
				JSON:      jsonOut,
				Region:    region,
			}
			engine, err := newEngine(cmd.Context(), region)
			if err != nil {
				return err
			}
			if cronSpec != "" {
				return schedule.Run(cmd.Context(), cronSpec, engine, opt, cmd.OutOrStdout())
			}
			rep, err := engine.Run(cmd.Context(), opt)
			if err != nil {
				return err
			}
			if jsonOut {
				return report.WriteJSON(cmd.OutOrStdout(), rep)
			}
			return report.WriteTable(cmd.OutOrStdout(), rep)
		},
	}
	applyFlags(scanCmd)
	scanCmd.Flags().StringVar(&cronSpec, "schedule", "", "cron expression; keep running and rescan")

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the last drift report and trigger on-demand scans",
		RunE: func(cmd *cobra.Command, args []string) error {
			opt := scan.Options{
				StatePath: statePath,
				BackendS3: backendS3,
				OutPath:   outPath,
				Unmanaged: unmanaged,
				Region:    region,
			}
			engine, err := newEngine(cmd.Context(), region)
			if err != nil {
				return err
			}
			srv := httpserver.New(engine, opt)
			fmt.Fprintf(cmd.OutOrStdout(), "dashboard listening on http://%s\n", listen)
			return srv.ListenAndServe(cmd.Context(), listen)
		},
	}
	applyFlags(serveCmd)
	serveCmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8080", "HTTP listen address")

	root := &cobra.Command{
		Use:   "tdd",
		Short: "Terraform drift detector — compare state to live infrastructure",
	}
	root.AddCommand(scanCmd, serveCmd)
	return root
}

func newEngine(ctx context.Context, region string) (*scan.Engine, error) {
	awsFetcher, err := awsp.New(ctx, region)
	if err != nil {
		return nil, err
	}
	reg := provider.NewRegistry(awsFetcher, provider.AzureStub(), provider.GCPStub())
	return scan.New(reg), nil
}
