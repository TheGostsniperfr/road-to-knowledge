---
name: Container runtimes
level: 2
state: progress
evidence: [https://github.com/opencontainers/runtime-spec]
---
Du kubelet au noyau : CRI, containerd, shim, runtime OCI.

```mermaid
flowchart TD
  kubelet -->|CRI gRPC| containerd
  containerd --> shim[shim v2]
  shim --> runc
  shim --> gvisor[runsc]
  runc --> kernel[(noyau Linux)]
```

CRI :: API gRPC entre kubelet et le runtime de conteneurs
shim v2 :: processus par pod entre containerd et le runtime

## OCI spec
Le contrat entre runtime et bundle : config.json, cycle de vie, hooks.

## CRI
API gRPC entre kubelet et le runtime de conteneurs.
