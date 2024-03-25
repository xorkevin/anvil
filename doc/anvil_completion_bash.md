## anvil completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(anvil completion bash)

To load completions for every new session, execute once:

#### Linux:

	anvil completion bash > /etc/bash_completion.d/anvil

#### macOS:

	anvil completion bash > $(brew --prefix)/etc/bash_completion.d/anvil

You will need to start a new shell for this setup to take effect.


```
anvil completion bash
```

### Options

```
  -h, --help              help for bash
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

