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
)

// secretCmd represents the secret command
var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Uploads secret configs",
	Long:  `Uploads secret configs`,
	Run: func(cmd *cobra.Command, args []string) {
		if secretVaultPolicies != "" {
			if err := vault.AddPolicies(context.Background(), secretVaultPolicies); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
		if secretVaultRoles != "" {
			if err := vault.AddRoles(context.Background(), secretVaultRoles); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(secretCmd)

	secretCmd.PersistentFlags().StringVar(&secretVaultPolicies, "vault-policies", "", "vault secret policies")
	secretCmd.PersistentFlags().StringVar(&secretVaultRoles, "vault-roles", "", "vault secret roles")
}
