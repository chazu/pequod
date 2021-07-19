# Install argocd
kubectl create ns argocd
kubectl apply -n argocd -f workloads/argocd/install.yaml
kubectl apply -n argocd -f workloads/argocd/project-production.yaml
# TODO The previous should be generic - the workload shouldn't care or know about env unless its in an overlay
# Create the relevant meta-app
kubectl apply -f meta-apps/production.yaml
