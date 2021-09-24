# Pequod

Bootstrappable Cloud Computing Laboratory

## Quick Start

```
make add-cluster-config
make bootstrap
make argo-password # Returns the admin argo password
# Port forward argocd here
argocd login localhost:8080
# Add your repository to argocd via Make target
make argo-add-config-repo


```

## Project Anatomy

```
├── bootstrap.sh -> Install argocd, create projects and self-manage argocd
├── meta-apps -> App of apps, one per environment
├── production
├── README.md
├── script -> Scripts, duh
├── staging
├── test -> Local test environment
└── workloads
    └── argocd
        └── install.yaml
```


## TODO
- Add terraform folder & scripts for terraform (for building cluster)
- Script/make target to get argocd admin password
- Script/make target to bootstrap server with k3s, retrieve config and add to kubeconfig file
- Automate addition of repository for argocd
- Automate test environment setup using kind
- Parameterize ingress and other relevant details when bootstrapping
- Start adding workloads, possibly adding tooling to support simple configuration
depending on use case
- Add workload: crossplane
- Add workload: OAM
- Add workload: cert-manager
- Add workload: osiris
- Add workload: argo workflows/events
- Add workload: earthly/drone
- Add workload: grafana/prometheus

## DONE
- add .gitignore
- add bootstrap folder to install gitops operator (argocd) incl. projects
- Add meta-apps folder to hold apps of apps - .e.g. production, etc
