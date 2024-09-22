```sh
kind create cluster --config kind-cluster-configs.yaml

docker build -t jehadnasser/namespace-protection-webhook:1.0.1 .

docker push  jehadnasser/namespace-protection-webhook:1.0.1

kind load docker-image jehadnasser/namespace-protection-webhook:1.0.1 --name demo-validating-webhook-protect-ns

k apply -f shield-operator-manifests/ns.yaml
k apply -f shield-operator-manifests/sa.yaml -f shield-operator-manifests/tls-secret.yaml -f shield-operator-manifests/cm.yaml
k apply -f shield-operator-manifests

```