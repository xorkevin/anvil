## anvil completion zsh

generate the autocompletion script for zsh

### Synopsis


Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

$ echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions for every new session, execute once:
# Linux:
$ anvil completion zsh > "${fpath[1]}/_anvil"
# macOS:
$ anvil completion zsh > /usr/local/share/zsh/site-functions/_anvil

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
      --config string   config file (default is $XDG_CONFIG_HOME/.anvil.yaml)
      --debug           turn on debug output
```

### SEE ALSO

* [anvil completion](anvil_completion.md)	 - generate the autocompletion script for the specified shell

