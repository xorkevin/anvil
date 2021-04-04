package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// componentCmd represents the component command
var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Prints component configs",
	Long:  `Prints component configs`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("hello, world")
	},
}

func init() {
	rootCmd.AddCommand(componentCmd)
}
