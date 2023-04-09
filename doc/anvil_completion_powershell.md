## anvil completion powershell

Generate the autocompletion script for powershell

### Synopsis

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	anvil completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.


```
anvil completion powershell [flags]
```

### Options

```
  -h, --help              help for powershell
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --config string      config file (default is $XDG_CONFIG_HOME/anvil/anvil.yaml)
      --log-json           output json logs
      --log-level string   log level (default "info")
```

### SEE ALSO

* [anvil completion](anvil_completion.md)	 - Generate the autocompletion script for the specified shell

