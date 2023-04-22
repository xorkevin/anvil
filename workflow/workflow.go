package workflow

import (
	"context"
	"io/fs"

	"xorkevin.dev/klog"
)

type (
	Opts struct{}
)

func ExecWorkflow(ctx context.Context, log klog.Logger, fsys fs.FS, opts Opts) error {
	return nil
}

func Exec(ctx context.Context, log klog.Logger, dir string, opts Opts) error {
	return nil
}
