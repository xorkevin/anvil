package cmd

import (
	"context"
	"log"

	"github.com/spf13/cobra"
	"xorkevin.dev/anvil/component"
)

type (
	componentFlags struct {
		output          string
		local           string
		remote          string
		config          string
		patch           string
		noNetwork       bool
		gitPartialClone bool
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
	componentCmd.PersistentFlags().StringVarP(&c.componentFlags.local, "input", "i", ".", "component definition directory")
	componentCmd.PersistentFlags().StringVarP(&c.componentFlags.remote, "remote-cache", "r", ".anvil_remote_cache", "remote component cache directory")
	componentCmd.PersistentFlags().StringVarP(&c.componentFlags.config, "component", "c", "component.yaml", "root component file")
	componentCmd.PersistentFlags().StringVarP(&c.componentFlags.patch, "patch", "p", "", "component patch file")
	componentCmd.PersistentFlags().BoolVar(&c.componentFlags.noNetwork, "no-network", false, "use cache only for remote components")
	componentCmd.PersistentFlags().BoolVar(&c.componentFlags.gitPartialClone, "git-partial-clone", false, "use git partial clone for remote git components")

	return componentCmd
}

func (c *Cmd) execComponentCmd(cmd *cobra.Command, args []string) {
	if err := component.Generate(
		context.Background(),
		c.componentFlags.output,
		c.componentFlags.local,
		c.componentFlags.remote,
		c.componentFlags.config,
		c.componentFlags.patch,
		component.Opts{
			NoNetwork:       c.componentFlags.noNetwork,
			GitPartialClone: c.componentFlags.gitPartialClone,
		},
	); err != nil {
		log.Fatalln(err)
	}
}
