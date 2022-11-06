package cmd

import (
	"context"
	"log"

	"github.com/spf13/cobra"
	"xorkevin.dev/anvil/secret/vault"
)

type (
	secretFlags struct {
		vaultPolicies string
		vaultRoles    string
		verbose       bool
		dryRun        bool
	}
)

func (c *Cmd) getSecretCmd() *cobra.Command {
	secretCmd := &cobra.Command{
		Use:               "secret",
		Short:             "Uploads secret configs",
		Long:              `Uploads secret configs`,
		Run:               c.execSecretCmd,
		DisableAutoGenTag: true,
	}
	secretCmd.PersistentFlags().StringVar(&c.secretFlags.vaultPolicies, "vault-policies", "", "vault secret policies")
	secretCmd.PersistentFlags().StringVar(&c.secretFlags.vaultRoles, "vault-roles", "", "vault secret roles")
	secretCmd.PersistentFlags().BoolVarP(&c.secretFlags.verbose, "verbose", "v", false, "verbose output")
	secretCmd.PersistentFlags().BoolVarP(&c.secretFlags.dryRun, "dry-run", "n", false, "dry run")

	return secretCmd
}

func (c *Cmd) execSecretCmd(cmd *cobra.Command, args []string) {
	opts := vault.Opts{
		Verbose: c.secretFlags.verbose,
		DryRun:  c.secretFlags.dryRun,
	}
	if c.secretFlags.vaultPolicies != "" {
		if err := vault.AddPolicies(context.Background(), c.secretFlags.vaultPolicies, opts); err != nil {
			log.Fatalln(err)
		}
	}
	if c.secretFlags.vaultRoles != "" {
		if err := vault.AddRoles(context.Background(), c.secretFlags.vaultRoles, opts); err != nil {
			log.Fatalln(err)
		}
	}
}
