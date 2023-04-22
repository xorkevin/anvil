package cmd

import (
	"context"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"xorkevin.dev/anvil/workflow"
	"xorkevin.dev/klog"
)

type (
	workflowFlags struct {
		input string
		opts  workflow.Opts
	}
)

func (c *Cmd) getWorkflowCmd() *cobra.Command {
	workflowCmd := &cobra.Command{
		Use:               "workflow",
		Short:             "Runs workflows",
		Long:              `Runs workflows`,
		Run:               c.execWorkflowCmd,
		DisableAutoGenTag: true,
	}
	workflowCmd.PersistentFlags().StringVarP(&c.workflowFlags.input, "input", "i", "", "workflow script")
	workflowCmd.PersistentFlags().IntVarP(&c.workflowFlags.opts.MaxRetries, "max-retries", "r", 10, "max workflow retries")
	workflowCmd.PersistentFlags().DurationVarP(&c.workflowFlags.opts.MinBackoff, "min-backoff", "l", time.Second, "min retry backoff")
	workflowCmd.PersistentFlags().DurationVarP(&c.workflowFlags.opts.MaxBackoff, "max-backoff", "h", 10*time.Second, "max retry backoff")

	return workflowCmd
}

func (c *Cmd) execWorkflowCmd(cmd *cobra.Command, args []string) {
	if err := workflow.Exec(
		context.Background(),
		c.log.Logger.Sublogger("", klog.AString("cmd", "workflow")),
		filepath.ToSlash(c.workflowFlags.input),
		c.workflowFlags.opts,
	); err != nil {
		c.logFatal(err)
		return
	}
}
