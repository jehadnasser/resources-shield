#!/bin/bash

set -e  # Exit immediately if a command exits with a non-zero status

VERSION=1.0.2
IMG_NAME=jehadnasser/resources-shield:$VERSION
CLUSTER_NAME=demo-validating-webhook-protect-ns

kind delete cluster --name $CLUSTER_NAME

sed -i '' "s|^\([[:space:]]*name:\).*|\1 $CLUSTER_NAME|" kind-cluster-configs.yaml

kind create cluster --config kind-cluster-configs.yaml

docker build -t $IMG_NAME .

docker push  $IMG_NAME

kind load docker-image $IMG_NAME --name $CLUSTER_NAME

./gen-tls.sh

sed -i '' "s|^\([[:space:]]*image:\).*|\1 $IMG_NAME|" manifests/deploy.yaml

kubectl apply -f manifests/ns.yaml
kubectl apply -f manifests/sa.yaml -f manifests/tls-secret.yaml -f manifests/cm.yaml
kubectl apply -f manifests