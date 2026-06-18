# multi-john

Run [John the Ripper](https://github.com/openwall/john), but coordinated on many machines.

## Image

Sporadic releases on Docker hub; `praktiskt/multi-john:latest`.

## Helm chart

The intended deployment path is Kubernetes via the Helm chart. See the [helm directory](./helm).

## How it works

`multi-john` installs a small Kubernetes control plane:

- `etcd` - stores worker status and cracked results.
- `howdy` - exposes a Web UI and API for submitting jobs and viewing results.
- `worker` - runs inside per-crack Kubernetes Indexed Jobs created by `howdy`.

For each run, `howdy` creates a Kubernetes Secret containing the hash input and an Indexed Job with one worker shard per completion index. Workers mount the Secret, run John the Ripper with `--node=N/M`, and publish potfile results back to etcd under that run id.
