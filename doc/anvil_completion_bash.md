## anvil completion bash

generate the autocompletion script for bash

### Synopsis


Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:
$ source <(anvil completion bash)

To load completions for every new session, execute once:
Linux:
  $ anvil completion bash > /etc/bash_completion.d/anvil
MacOS:
  $ anvil completion bash > /usr/local/etc/bash_completion.d/anvil

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
      --config string   config file (default is $XDG_CONFIG_HOME/.anvil.yaml)
      --debug           turn on debug output
```

### SEE ALSO

* [anvil completion](anvil_completion.md)	 - generate the autocompletion script for the specified shell

