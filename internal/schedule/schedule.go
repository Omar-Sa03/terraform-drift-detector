package schedule

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/medom/terraform-drift-detector/internal/report"
	"github.com/medom/terraform-drift-detector/internal/scan"
	"github.com/robfig/cron/v3"
)

func Run(ctx context.Context, spec string, engine *scan.Engine, opt scan.Options, stdout io.Writer) error {
	c := cron.New()
	_, err := c.AddFunc(spec, func() {
		rep, err := engine.Run(ctx, opt)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scheduled scan failed: %v\n", err)
			return
		}
		if opt.JSON {
			_ = report.WriteJSON(stdout, rep)
		} else {
			_ = report.WriteTable(stdout, rep)
		}
	})
	if err != nil {
		return fmt.Errorf("invalid cron spec %q: %w", spec, err)
	}
	c.Start()
	fmt.Fprintf(stdout, "scheduled drift scans with %q (ctrl+c to stop)\n", spec)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-ctx.Done():
	case <-sig:
	}
	ctxStop := c.Stop()
	<-ctxStop.Done()
	return nil
}
