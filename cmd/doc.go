package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

type (
	docFlags struct {
		docOutputDir string
	}
)

func (c *Cmd) getDocCmd() *cobra.Command {
	docCmd := &cobra.Command{
		Use:               "doc",
		Short:             "generate documentation for anvil",
		Long:              `generate documentation for anvil in several formats`,
		DisableAutoGenTag: true,
	}
	docCmd.PersistentFlags().StringVarP(&c.docFlags.docOutputDir, "output", "o", ".", "documentation output path")

	docManCmd := &cobra.Command{
		Use:               "man",
		Short:             "generate man page documentation for anvil",
		Long:              `generate man page documentation for anvil`,
		Run:               c.execDocManCmd,
		DisableAutoGenTag: true,
	}
	docCmd.AddCommand(docManCmd)

	docMdCmd := &cobra.Command{
		Use:               "md",
		Short:             "generate markdown documentation for anvil",
		Long:              `generate markdown documentation for anvil`,
		Run:               c.execDocMdCmd,
		DisableAutoGenTag: true,
	}
	docCmd.AddCommand(docMdCmd)

	return docCmd
}

func (c *Cmd) execDocManCmd(cmd *cobra.Command, args []string) {
	if err := doc.GenManTree(c.rootCmd, &doc.GenManHeader{
		Title:   "anvil",
		Section: "1",
	}, c.docFlags.docOutputDir); err != nil {
		c.logFatal(err)
		return
	}
}

func (c *Cmd) execDocMdCmd(cmd *cobra.Command, args []string) {
	if err := doc.GenMarkdownTree(c.rootCmd, c.docFlags.docOutputDir); err != nil {
		c.logFatal(err)
		return
	}
}
