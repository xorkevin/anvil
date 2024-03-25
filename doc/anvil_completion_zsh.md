## anvil completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(anvil completion zsh)

To load completions for every new session, execute once:

#### Linux:

	anvil completion zsh > "${fpath[1]}/_anvil"

#### macOS:

	anvil completion zsh > $(brew --prefix)/share/zsh/site-functions/_anvil

You will need to start a new shell for this setup to take effect.


```
anvil completion zsh [flags]
```

### Options

```
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --config string      config file (default is $XDG_CONFIG_HOME/anvil/anvil.json)
      --log-json           output json logs
      --log-level string   log level (default "info")
```

### SEE ALSO

* [anvil completion](anvil_completion.md)	 - Generate the autocompletion script for the specified shell

