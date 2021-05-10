package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"xorkevin.dev/anvil/component"
)

var (
	componentOutput          string
	componentLocal           string
	componentRemote          string
	componentConfig          string
	componentPatch           string
	componentNoNetwork       bool
	componentGitPartialClone bool
)

// componentCmd represents the component command
var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Prints component configs",
	Long:  `Prints component configs`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := component.Generate(
			context.Background(),
			componentOutput,
			componentLocal,
			componentRemote,
			componentConfig,
			componentPatch,
			component.Opts{
				NoNetwork:       componentNoNetwork,
				GitPartialClone: componentGitPartialClone,
			},
		); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(componentCmd)

	componentCmd.PersistentFlags().StringVarP(&componentOutput, "output", "o", "anvil_out", "generated component output directory")
	componentCmd.PersistentFlags().StringVarP(&componentLocal, "input", "i", ".", "component definition directory")
	componentCmd.PersistentFlags().StringVarP(&componentRemote, "remote-cache", "r", ".anvil_remote_cache", "remote component cache directory")
	componentCmd.PersistentFlags().StringVarP(&componentConfig, "config", "c", "component.yaml", "root component file")
	componentCmd.PersistentFlags().StringVarP(&componentPatch, "patch", "p", "patch.yaml", "component patch file")
	componentCmd.PersistentFlags().BoolVar(&componentNoNetwork, "no-network", false, "use cache only for remote components")
	componentCmd.PersistentFlags().BoolVar(&componentGitPartialClone, "git-partial-clone", false, "use git partial clone for remote git components")
}
