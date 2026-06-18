# Helm chart for multi-john
Using this Helm chart, you can run multi-john on a Kubernetes cluster.

## Images
By default, the chart pins the current published multi-john image, `praktiskt/multi-john:bf184dba2942cda128b07e46bd54e13e30094c4f`, instead of the floating `latest` tag.

The embedded coordination store uses the official etcd image `quay.io/coreos/etcd:v3.6.11`.

Openwall's `ghcr.io/openwall/john` images contain John the Ripper itself; they are not a drop-in replacement for `multijohn.imageName` because the chart container must also include the `multijohn` wrapper binary.

## Usage
### tldr
```shell
kubectl create namespace <namespace>
helm install multi-john . --namespace <namespace> --values values.yaml
kubectl port-forward -n <namespace> service/howdy 8080:8080
```

### How it works
1. Configure `values.yaml`.
    * `.multijohn.imageName` is the image used by both the Web UI/controller and worker Jobs.
    * `.multijohn.totalNodes` is the default shard count shown by the Web UI.
    * `.multijohn.node.johnFlags` is the default John flag string shown by the Web UI.
    * `.multijohn.node.requests` and `.multijohn.node.limits` are copied into worker Jobs.
2. Install the chart.
```shell
kubectl create namespace <namespace>
helm install <name> . -n <namespace> --values values.yaml
```
3. Open the Web UI.
```shell
kubectl port-forward -n <namespace> service/howdy 8080:8080
```
4. Submit hash input in the UI. The controller creates a Secret for the hash file and an Indexed Job for the workers. Optionally set a node selector such as `nodepool=cpu-workers` to constrain the worker Pods.
5. Once you're satisfied with the results, uninstall the chart.
```shell
helm uninstall <name> -n <namespace>
```
