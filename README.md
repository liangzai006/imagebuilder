### for development:
```bash
GOOS=linux make manifests
```

### install the CRD and the controller:
```bash
kubectl apply -f config/crd/imagebuilder.ai.qingcloud.com_imagebuilders.yaml
kubectl apply -f deploy/install.yaml
```

### test:

```bash
kubectl apply -f config/sample/test.yaml
```

