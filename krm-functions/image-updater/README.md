## Usage

> transformer.yaml

```yaml
apiVersion: v1
kind: AutoUpdateImageTransformer
metadata:
  name: auto-update-image
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: image-updater
spec:
  targetImgSelector:
    pattern: ^quay.io/.+$
  remoteTagSelector:
    pattern: ^master-(?P<commit>.+)-(?P<timestamp>.+)$
    extract: timestamp
    sort: numerically
    order: desc
```

> kustomization.yaml

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

transformers:
  - transformer.yaml
```
