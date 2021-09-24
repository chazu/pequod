#!/bin/sh

DEPLOY_ENV=production

kubectl create ns argocd
# Install argocd
kubectl apply -n argocd -f workloads/argocd/base/install.yaml
# Create the relevant meta-app
kubectl apply -f meta-apps/${DEPLOY_ENV}.yaml
