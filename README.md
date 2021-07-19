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
- add bootstrap folder to install gitops operator (argocd) incl. projects
- Add meta-apps folder to hold apps of apps - .e.g. production, etc
- Add production folder to hold app instances for production
- Add other environments as necessary (staging I guess)
- Add test environment using kind? iono...
- Add terraform folder & scripts

## DONE
- add .gitignore
