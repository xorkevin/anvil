package workflow

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	"xorkevin.dev/anvil/workflowengine"
	"xorkevin.dev/anvil/workflowengine/starlarkengine"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	wfKindStarlark = "starlark"
)

type (
	Opts struct {
		MaxRetries      int
		MinBackoff      time.Duration
		MaxBackoff      time.Duration
		StarlarkLibName string
	}
)

func ExecWorkflow(ctx context.Context, log klog.Logger, algs workflowengine.Map, fsys fs.FS, kind string, name string, stdout io.Writer, opts Opts) error {
	eng, err := algs.Build(kind, fsys)
	if err != nil {
		return kerrors.WithMsg(err, fmt.Sprintf("Failed to build %s workflow engine", kind))
	}
	if _, err := workflowengine.ExecWorkflow(ctx, eng, name, nil, workflowengine.WorkflowOpts{
		Log:        log,
		Stdout:     stdout,
		MaxRetries: opts.MaxRetries,
		MinBackoff: opts.MinBackoff,
		MaxBackoff: opts.MaxBackoff,
	}); err != nil {
		return kerrors.WithMsg(err, fmt.Sprintf("Failed running %s workflow: %s", kind, name))
	}
	return nil
}

func Exec(ctx context.Context, log klog.Logger, input string, opts Opts) error {
	local, name := path.Split(input)
	local = path.Clean(local)
	name = path.Clean(name)
	return ExecWorkflow(ctx, log, workflowengine.Map{
		wfKindStarlark: starlarkengine.Builder{starlarkengine.OptLibName(opts.StarlarkLibName)},
	}, os.DirFS(filepath.FromSlash(local)), wfKindStarlark, name, os.Stderr, opts)
}
