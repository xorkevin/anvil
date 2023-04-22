## anvil component

Prints component configs

### Synopsis

Prints component configs

```
anvil component [flags]
```

### Options

```
  -c, --cache string            repo cache directory
  -n, --dry-run                 dry run writing components
  -f, --force-fetch             force refetching repos regardless of cache
      --git-cmd string          git cmd (default "git")
      --git-cmd-quiet           quiet git cmd output
      --git-dir string          git repo dir (.git) (default ".git")
  -h, --help                    help for component
  -i, --input string            main component definition
      --jsonnet-stdlib string   jsonnet std lib import name (default "anvil:std")
  -m, --no-network              error if the network is required
  -o, --output string           generated component output directory (default "anvil_out")
      --repo-sum string         checksum file (default "anvil.sum.json")
```

### Options inherited from parent commands

```
      --config string      config file (default is $XDG_CONFIG_HOME/anvil/anvil.yaml)
      --log-json           output json logs
      --log-level string   log level (default "info")
```

### SEE ALSO

* [anvil](anvil.md)	 - A compositional template generator

