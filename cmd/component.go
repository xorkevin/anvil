package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"xorkevin.dev/anvil/component"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	componentFlags struct {
		output string
		input  string
		cache  string
		opts   component.Opts
	}
)

func (c *Cmd) getComponentCmd() *cobra.Command {
	componentCmd := &cobra.Command{
		Use:               "component",
		Short:             "Prints component configs",
		Long:              `Prints component configs`,
		Run:               c.execComponentCmd,
		DisableAutoGenTag: true,
	}
	componentCmd.PersistentFlags().StringVarP(&c.componentFlags.output, "output", "o", "anvil_out", "generated component output directory")
	componentCmd.PersistentFlags().StringVarP(&c.componentFlags.input, "input", "i", ".", "main component definition")
	componentCmd.PersistentFlags().StringVarP(&c.componentFlags.cache, "cache", "c", "", "repo cache directory")
	componentCmd.PersistentFlags().BoolVarP(&c.componentFlags.opts.DryRun, "dry-run", "n", false, "dry run writing components")
	componentCmd.PersistentFlags().BoolVarP(&c.componentFlags.opts.NoNetwork, "no-network", "m", false, "error if the network is required")
	componentCmd.PersistentFlags().BoolVarP(&c.componentFlags.opts.ForceFetch, "force-fetch", "f", false, "force refetching repos regardless of cache")
	componentCmd.PersistentFlags().StringVar(&c.componentFlags.opts.RepoChecksumFile, "repo-sum", "anvil.sum.json", "checksum file")
	componentCmd.PersistentFlags().StringVar(&c.componentFlags.opts.GitDir, "git-dir", ".git", "git repo dir (.git)")
	componentCmd.PersistentFlags().StringVar(&c.componentFlags.opts.GitBin, "git-cmd", "git", "git cmd")
	componentCmd.PersistentFlags().BoolVar(&c.componentFlags.opts.GitBinQuiet, "git-cmd-quiet", false, "quiet git cmd output")
	componentCmd.PersistentFlags().StringVar(&c.componentFlags.opts.JsonnetLibName, "jsonnet-stdlib", "anvil:std", "jsonnet std lib import name")

	viper.SetDefault("component.repocache", "")

	return componentCmd
}

func (c *Cmd) execComponentCmd(cmd *cobra.Command, args []string) {
	cache := c.componentFlags.cache
	if cache == "" {
		cache = viper.GetString("component.repocache")
	}
	if cache == "" {
		if cachedir, err := os.UserCacheDir(); err != nil {
			c.log.WarnErr(context.Background(), kerrors.WithMsg(err, "Failed reading user cache dir"))
		} else {
			cache = filepath.Join(cachedir, "anvil", "repo")
		}
	}
	if cache == "" {
		cache = filepath.Join(".anvil", "cache", "repo")
	}
	if err := component.Generate(
		context.Background(),
		c.log.Logger.Sublogger("", klog.AString("cmd", "component")),
		filepath.ToSlash(c.componentFlags.output),
		filepath.ToSlash(c.componentFlags.input),
		filepath.ToSlash(cache),
		c.componentFlags.opts,
	); err != nil {
		c.logFatal(err)
		return
	}
}
