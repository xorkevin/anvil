## anvil component

Prints component configs

### Synopsis

Prints component configs

```
anvil component [flags]
```

### Options

```
  -c, --component string      root component file (default "component.yaml")
      --git-partial-clone     use git partial clone for remote git components
  -h, --help                  help for component
  -i, --input string          component definition directory (default ".")
      --no-network            use cache only for remote components
  -o, --output string         generated component output directory (default "anvil_out")
  -p, --patch string          component patch file
  -r, --remote-cache string   remote component cache directory (default ".anvil_remote_cache")
```

### Options inherited from parent commands

```
      --config string   config file (default is $XDG_CONFIG_HOME/.anvil.yaml)
      --debug           turn on debug output
```

### SEE ALSO

* [anvil](anvil.md)	 - A compositional template generator

