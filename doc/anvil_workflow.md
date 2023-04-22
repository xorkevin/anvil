## anvil workflow

Runs workflows

### Synopsis

Runs workflows

```
anvil workflow [flags]
```

### Options

```
  -h, --help                     help for workflow
  -i, --input string             workflow script
  -m, --max-backoff duration     max retry backoff (default 10s)
  -r, --max-retries int          max workflow retries (default 10)
  -l, --min-backoff duration     min retry backoff (default 1s)
      --starlark-stdlib string   starlark std lib import name (default "anvil:std")
```

### Options inherited from parent commands

```
      --config string      config file (default is $XDG_CONFIG_HOME/anvil/anvil.yaml)
      --log-json           output json logs
      --log-level string   log level (default "info")
```

### SEE ALSO

* [anvil](anvil.md)	 - A compositional template generator

