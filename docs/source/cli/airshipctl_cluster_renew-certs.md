## airshipctl cluster renew-certs

Renew control plane certificates close to expiration and restart control plane components

### Synopsis

Renews control plane certificates if the expiration threshold is met followed by restarting control plane components.


```
airshipctl cluster renew-certs [flags]
```

### Examples

```

  airshipctl cluster renew-certs

```

### Options

```
  -h, --help   help for renew-certs
```

### Options inherited from parent commands

```
      --airshipconf string   Path to file for airshipctl configuration. (default "$HOME/.airship/config")
      --debug                enable verbose output
      --kubeconfig string    Path to kubeconfig associated with airshipctl configuration. (default "$HOME/.airship/kubeconfig")
```

### SEE ALSO

* [airshipctl cluster](airshipctl_cluster.md)	 - Manage Kubernetes clusters

