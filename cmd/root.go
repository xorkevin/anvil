package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	Cmd struct {
		rootCmd        *cobra.Command
		log            *klog.LevelLogger
		version        string
		rootFlags      rootFlags
		componentFlags componentFlags
		secretFlags    secretFlags
		workflowFlags  workflowFlags
		docFlags       docFlags
	}

	rootFlags struct {
		cfgFile  string
		logLevel string
		logJSON  bool
	}
)

func New() *Cmd {
	return &Cmd{}
}

func (c *Cmd) Execute() {
	buildinfo := ReadVCSBuildInfo()
	c.version = buildinfo.ModVersion
	rootCmd := &cobra.Command{
		Use:               "anvil",
		Short:             "A compositional template generator",
		Long:              `A compositional template generator`,
		Version:           c.version,
		PersistentPreRun:  c.initConfig,
		DisableAutoGenTag: true,
	}
	rootCmd.PersistentFlags().StringVar(&c.rootFlags.cfgFile, "config", "", "config file (default is $XDG_CONFIG_HOME/anvil/anvil.json)")
	rootCmd.PersistentFlags().StringVar(&c.rootFlags.logLevel, "log-level", "info", "log level")
	rootCmd.PersistentFlags().BoolVar(&c.rootFlags.logJSON, "log-json", false, "output json logs")
	c.rootCmd = rootCmd

	rootCmd.AddCommand(c.getComponentCmd())
	rootCmd.AddCommand(c.getSecretCmd())
	rootCmd.AddCommand(c.getWorkflowCmd())
	rootCmd.AddCommand(c.getDocCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
		return
	}
}

// initConfig reads in config file and ENV variables if set.
func (c *Cmd) initConfig(cmd *cobra.Command, args []string) {
	logWriter := klog.NewSyncWriter(os.Stderr)
	var handler *klog.SlogHandler
	if c.rootFlags.logJSON {
		handler = klog.NewJSONSlogHandler(logWriter)
	} else {
		handler = klog.NewTextSlogHandler(logWriter)
		handler.FieldTime = ""
		handler.FieldCaller = ""
		handler.FieldMod = ""
	}
	c.log = klog.NewLevelLogger(klog.New(
		klog.OptHandler(handler),
		klog.OptMinLevelStr(c.rootFlags.logLevel),
	))

	if c.rootFlags.cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(c.rootFlags.cfgFile)
	} else {
		viper.SetConfigName("anvil")
		viper.AddConfigPath(".")

		// Search config in $XDG_CONFIG_HOME/anvil directory
		if cfgdir, err := os.UserConfigDir(); err != nil {
			c.log.WarnErr(context.Background(), kerrors.WithMsg(err, "Failed reading user config dir"))
		} else {
			viper.AddConfigPath(filepath.Join(cfgdir, "anvil"))
		}
	}

	viper.SetEnvPrefix("ANVIL")
	viper.AutomaticEnv() // read in environment variables that match
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "__"))

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		c.log.Debug(context.Background(), "Failed reading config", klog.AString("err", err.Error()))
	} else {
		c.log.Debug(context.Background(), "Using config", klog.AString("file", viper.ConfigFileUsed()))
	}
}

func (c *Cmd) logFatal(err error) {
	c.log.Err(context.Background(), err)
	os.Exit(1)
}
