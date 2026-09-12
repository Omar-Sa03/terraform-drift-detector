package scan

import (
	"context"
	"fmt"

	"github.com/medom/terraform-drift-detector/internal/compare"
	"github.com/medom/terraform-drift-detector/internal/model"
	"github.com/medom/terraform-drift-detector/internal/provider"
	"github.com/medom/terraform-drift-detector/internal/report"
	"github.com/medom/terraform-drift-detector/internal/state"
)

type Options struct {
	StatePath string
	BackendS3 string
	OutPath   string
	Unmanaged bool
	JSON      bool
	Region    string
}

type Engine struct {
	Loader   *state.Loader
	Registry *provider.Registry
}

func New(reg *provider.Registry) *Engine {
	return &Engine{Loader: state.NewLoader(), Registry: reg}
}

func (e *Engine) Run(ctx context.Context, opt Options) (model.Report, error) {
	if e.Loader == nil {
		e.Loader = state.NewLoader()
	}
	expected, src, err := e.Loader.Load(ctx, opt.StatePath, opt.BackendS3)
	if err != nil {
		return model.Report{}, err
	}
	actual, warnings, err := e.Registry.FetchAll(ctx, expected)
	if err != nil {
		return model.Report{}, fmt.Errorf("fetch live resources: %w", err)
	}
	drifts := compare.Diff(expected, actual, compare.Options{Unmanaged: opt.Unmanaged})
	rep := report.Build(src, expected, actual, drifts, warnings)
	if opt.OutPath != "" {
		if err := report.SaveFile(opt.OutPath, rep); err != nil {
			return rep, fmt.Errorf("save report: %w", err)
		}
	}
	return rep, nil
}
