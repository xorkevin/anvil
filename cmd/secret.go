package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"xorkevin.dev/anvil/secret/vault"
	"xorkevin.dev/klog"
)

type (
	secretFlags struct {
		vault vaultFlags
	}

	vaultFlags struct {
		policies string
		roles    string
		opts     vault.Opts
	}
)

func (c *Cmd) getSecretCmd() *cobra.Command {
	secretCmd := &cobra.Command{
		Use:               "secret",
		Short:             "Uploads secret configs",
		Long:              `Uploads secret configs`,
		DisableAutoGenTag: true,
	}

	vaultCmd := &cobra.Command{
		Use:               "vault",
		Short:             "Uploads vault configs",
		Long:              `Uploads vault configs`,
		Run:               c.execSecretVaultCmd,
		DisableAutoGenTag: true,
	}
	vaultCmd.PersistentFlags().StringVar(&c.secretFlags.vault.policies, "policies", "", "vault secret policies")
	vaultCmd.PersistentFlags().StringVar(&c.secretFlags.vault.roles, "roles", "", "vault secret roles")
	vaultCmd.PersistentFlags().BoolVarP(&c.secretFlags.vault.opts.DryRun, "dry-run", "n", false, "dry run uploading policies and roles")
	secretCmd.AddCommand(vaultCmd)

	return secretCmd
}

func (c *Cmd) execSecretVaultCmd(cmd *cobra.Command, args []string) {
	if c.secretFlags.vault.policies != "" {
		if err := vault.AddPolicies(
			context.Background(),
			c.log.Logger.Sublogger("", klog.AString("cmd", "secret.vault")),
			c.secretFlags.vault.policies,
			c.secretFlags.vault.opts,
		); err != nil {
			c.logFatal(err)
			return
		}
	}
	if c.secretFlags.vault.roles != "" {
		if err := vault.AddRoles(
			context.Background(),
			c.log.Logger.Sublogger("", klog.AString("cmd", "secret.vault")),
			c.secretFlags.vault.roles,
			c.secretFlags.vault.opts,
		); err != nil {
			c.logFatal(err)
			return
		}
	}
}
