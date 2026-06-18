# multi-john chart

Installs the multi-john Web UI/controller and etcd. Worker Pods are created per run as Kubernetes Indexed Jobs.

```shell
kubectl create namespace <namespace>
helm install multi-john ./charts/multi-john -n <namespace> --values charts/multi-john/values.yaml
kubectl port-forward -n <namespace> service/howdy 8080:8080
```

Default app image: `ghcr.io/adamanteye/multi-john:0.1.3`.

Open `http://localhost:8080` to submit hash input. The controller stores hashes in a Secret and creates an Indexed Job with the requested shard count, parallelism, and optional node selector.

The chart creates a shared workspace PVC by default:

```yaml
multijohn:
  work:
    enabled: true
    mountPath: /work
    accessModes:
      - ReadWriteMany
```

The same PVC is mounted into the controller and every worker Job. Set `multijohn.work.existingClaim` to reuse an existing PVC.

Values configure the controller, UI defaults, and worker runtime defaults. Hashes, shard count, John flags, and node selector are submitted per run through the controller.

Worker Jobs can be customized with a strategic merge patch applied to the generated Job `spec.template`:

```yaml
multijohn:
  worker:
    podTemplatePatch:
      spec:
        priorityClassName: batch
        affinity:
          nodeAffinity:
            requiredDuringSchedulingIgnoredDuringExecution:
              nodeSelectorTerms:
                - matchExpressions:
                    - key: nodepool
                      operator: In
                      values:
                        - cpu
```
