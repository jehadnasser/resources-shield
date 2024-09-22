# Resources-Shield
This Operator protects namespaces in your cluster, and protect its own resources as well

## How it works
to start the test run this script:
```sh
./bootstrap.sh
```

## How to protect your own namespaces
- Edit this ConfigMap in `mainfests/cm.yaml`
    ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
        name: resources-shield-cm
        namespace: resources-shield
    data:
        protected-resources-shield-list.yaml: |
            protectedNamespaces:
                - kube-system
                - default
                - my-important-namespace
                - kyverno
                - flux-system
                - crossplane-system
                - castai-system
                - karpenter
                - cert-manager
                - calico
                # add your own namespaces
    ```
- Apply your changes and it will automatically upate the operator memory with the new namespaces:
    ```sh
    kubectl apply -f mainfests/cm.yaml
    ```

## What's next?
- find a way to stop this operator when I need to really delete?
- does fluxCD will keep them exists? or do I need a specific controller for this operator?