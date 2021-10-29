package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"xorkevin.dev/anvil/secret/vault"
)

var (
	secretVaultPolicies string
	secretVaultRoles    string
	secretVerbose       bool
	secretDryRun        bool
)

// secretCmd represents the secret command
var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Uploads secret configs",
	Long:  `Uploads secret configs`,
	Run: func(cmd *cobra.Command, args []string) {
		opts := vault.Opts{
			Verbose: secretVerbose,
			DryRun:  secretDryRun,
		}
		if secretVaultPolicies != "" {
			if err := vault.AddPolicies(context.Background(), secretVaultPolicies, opts); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
		if secretVaultRoles != "" {
			if err := vault.AddRoles(context.Background(), secretVaultRoles, opts); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
	},
	DisableAutoGenTag: true,
}

func init() {
	rootCmd.AddCommand(secretCmd)

	secretCmd.PersistentFlags().StringVar(&secretVaultPolicies, "vault-policies", "", "vault secret policies")
	secretCmd.PersistentFlags().StringVar(&secretVaultRoles, "vault-roles", "", "vault secret roles")
	secretCmd.PersistentFlags().BoolVarP(&secretVerbose, "verbose", "v", false, "verbose output")
	secretCmd.PersistentFlags().BoolVarP(&secretDryRun, "dry-run", "n", false, "dry run")
}
