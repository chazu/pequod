# Pequod

Bootstrappable Cloud Computing Laboratory

## Quick Start

```

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
- Script/make target to get argocd admin password
- Automate addition of repository for argocd
- Add terraform folder & scripts for terraform
- Automate test environment setup using kind
- Parameterize ingress and other relevant details when bootstrapping
- Start adding workloads, possibly adding tooling to support simple configuration depending on use case

## DONE
- add .gitignore
- add bootstrap folder to install gitops operator (argocd) incl. projects
- Add meta-apps folder to hold apps of apps - .e.g. production, etc
