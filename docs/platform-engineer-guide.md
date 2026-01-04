# Platform Engineer Guide

This guide covers how to create, test, and distribute CUE platform modules for Pequod.

## Table of Contents

- [Overview](#overview)
- [Platform Module Structure](#platform-module-structure)
- [Creating a Platform Module](#creating-a-platform-module)
- [Schema Design](#schema-design)
- [Render Templates](#render-templates)
- [Policy Authoring](#policy-authoring)
- [Testing Platform Modules](#testing-platform-modules)
- [Versioning and Distribution](#versioning-and-distribution)

## Overview

Platform modules are CUE packages that define:

1. **Schema** (`schema.cue`): Input validation and types
2. **Render** (`render.cue`): Resource generation templates
3. **Policy** (`policy.cue`): Constraints and guardrails (optional)

Developers create `Transform` resources with inputs. Pequod evaluates the CUE module to produce a `ResourceGraph` containing Kubernetes resources with dependencies.

```
Transform (input) → CUE Module → ResourceGraph (output) → Kubernetes Resources
```

## Platform Module Structure

A typical platform module has this structure:

```
myplatform/
├── schema.cue      # Input schema and types
├── render.cue      # Resource templates and graph generation
├── policy.cue      # Policies and constraints (optional)
└── README.md       # Documentation
```

### Required Definitions

Every platform module must define:

1. **`#Render`** - The main render template that takes input and produces output

```cue
#Render: {
    input: {
        metadata: { name: string, namespace: string }
        spec: #YourInputSchema
    }
    output: #Graph
}
```

## Creating a Platform Module

### Step 1: Define the Input Schema

Create `schema.cue` with your input types:

```cue
package myplatform

// #MyPlatformSpec defines what users can configure
#MyPlatformSpec: {
    // Required fields
    image: string & !=""
    port:  int & >=1 & <=65535

    // Optional fields with defaults
    replicas?: int & >=0 & <=100

    // Constrained fields
    environment: *"development" | "staging" | "production"
}

// #MyPlatformInput is the complete input structure
#MyPlatformInput: {
    metadata: {
        name:      string
        namespace: string
    }
    spec: #MyPlatformSpec
}
```

### Step 2: Define the Output Graph Types

Add graph types to `render.cue`:

```cue
package myplatform

// #Graph is the output structure
#Graph: {
    metadata: {
        name:        string
        version:     "v1alpha1"
        platformRef: string
    }
    nodes:      [...#Node]
    violations: [...#Violation]
}

// #Node represents a Kubernetes resource
#Node: {
    id:     string
    object: _  // Any Kubernetes object
    applyPolicy: #ApplyPolicy
    dependsOn:   [...string]
    readyWhen:   [...#ReadinessPredicate]
}

// #ApplyPolicy defines how to apply the resource
#ApplyPolicy: {
    mode:           *"Apply" | "Create" | "Adopt"
    conflictPolicy: *"Error" | "Force"
    fieldManager?:  string
}

// #ReadinessPredicate defines when a resource is ready
#ReadinessPredicate: {
    type:             "ConditionMatch" | "DeploymentAvailable" | "Exists"
    conditionType?:   string
    conditionStatus?: string
}

// #Violation represents a policy violation
#Violation: {
    path:     string
    message:  string
    severity: *"Error" | "Warning"
}
```

### Step 3: Create the Render Template

Add the `#Render` definition:

```cue
package myplatform

#Render: {
    input: #MyPlatformInput

    output: #Graph & {
        metadata: {
            name:        "\(input.metadata.name)-graph"
            platformRef: "myplatform"
        }

        nodes: [
            // ConfigMap node
            {
                id: "configmap"
                object: {
                    apiVersion: "v1"
                    kind:       "ConfigMap"
                    metadata: {
                        name:      "\(input.metadata.name)-config"
                        namespace: input.metadata.namespace
                    }
                    data: {
                        PORT: "\(input.spec.port)"
                        ENV:  input.spec.environment
                    }
                }
                applyPolicy: {
                    mode: "Apply"
                }
                dependsOn: []
                readyWhen: [{ type: "Exists" }]
            },

            // Deployment node (depends on ConfigMap)
            {
                id: "deployment"
                object: {
                    apiVersion: "apps/v1"
                    kind:       "Deployment"
                    metadata: {
                        name:      input.metadata.name
                        namespace: input.metadata.namespace
                    }
                    spec: {
                        replicas: input.spec.replicas
                        selector: matchLabels: app: input.metadata.name
                        template: {
                            metadata: labels: app: input.metadata.name
                            spec: containers: [{
                                name:  input.metadata.name
                                image: input.spec.image
                                ports: [{ containerPort: input.spec.port }]
                                envFrom: [{
                                    configMapRef: name: "\(input.metadata.name)-config"
                                }]
                            }]
                        }
                    }
                }
                applyPolicy: { mode: "Apply" }
                dependsOn: ["configmap"]  // Wait for ConfigMap
                readyWhen: [{ type: "DeploymentAvailable" }]
            },

            // Service node (depends on Deployment)
            {
                id: "service"
                object: {
                    apiVersion: "v1"
                    kind:       "Service"
                    metadata: {
                        name:      input.metadata.name
                        namespace: input.metadata.namespace
                    }
                    spec: {
                        selector: app: input.metadata.name
                        ports: [{ port: input.spec.port, targetPort: input.spec.port }]
                    }
                }
                applyPolicy: { mode: "Apply" }
                dependsOn: ["deployment"]
                readyWhen: [{ type: "Exists" }]
            },
        ]

        violations: []
    }
}
```

## Schema Design

### Field Validation

CUE provides powerful validation:

```cue
#MySpec: {
    // Non-empty string
    name: string & !=""

    // Bounded integer
    replicas: int & >=1 & <=100

    // Enumeration
    size: "small" | "medium" | "large"

    // Optional with default
    debug?: bool | *false

    // Pattern matching
    version: =~"^v[0-9]+\\.[0-9]+\\.[0-9]+$"

    // Conditional fields
    if size == "large" {
        minCPU: int & >=4
    }
}
```

### Nested Types

```cue
#DatabaseSpec: {
    engine: "postgres" | "mysql"

    storage: {
        size:         string  // e.g., "10Gi"
        storageClass: string | *"standard"
    }

    backup?: {
        enabled:  bool | *true
        schedule: string | *"0 2 * * *"
    }
}
```

### Optional Fields with Conditional Rendering

```cue
#Render: {
    input: #Input

    output: #Graph & {
        nodes: [
            // Always include
            { id: "main", ... },

            // Conditionally include based on input
            if input.spec.backup != _|_ && input.spec.backup.enabled {
                { id: "backup-job", ... }
            },
        ]
    }
}
```

## Render Templates

### Node Dependencies

Use `dependsOn` to specify ordering:

```cue
nodes: [
    {
        id: "namespace"
        dependsOn: []  // No dependencies, created first
    },
    {
        id: "configmap"
        dependsOn: ["namespace"]  // Wait for namespace
    },
    {
        id: "deployment"
        dependsOn: ["configmap", "secret"]  // Wait for both
    },
]
```

### Readiness Predicates

Available predicate types:

```cue
// Check resource exists
readyWhen: [{ type: "Exists" }]

// Check Deployment has available replicas
readyWhen: [{ type: "DeploymentAvailable" }]

// Check specific condition
readyWhen: [{
    type:            "ConditionMatch"
    conditionType:   "Ready"
    conditionStatus: "True"
}]
```

### Apply Policies

```cue
applyPolicy: {
    // Mode: Apply (SSA), Create (only if not exists), Adopt (take ownership)
    mode: "Apply"

    // ConflictPolicy: Error (fail on conflict), Force (overwrite)
    conflictPolicy: "Error"

    // Optional custom field manager
    fieldManager: "my-platform"
}
```

## Policy Authoring

### Adding Violations

Add policy checks that produce violations:

```cue
package myplatform

#Render: {
    input: #Input

    _violations: [...#Violation]

    // Check image registry
    if !strings.HasPrefix(input.spec.image, "ghcr.io/myorg/") {
        _violations: _violations + [{
            path:     "spec.image"
            message:  "Image must be from ghcr.io/myorg/ registry"
            severity: "Error"
        }]
    }

    // Check production requirements
    if input.spec.environment == "production" && input.spec.replicas < 3 {
        _violations: _violations + [{
            path:     "spec.replicas"
            message:  "Production requires at least 3 replicas"
            severity: "Error"
        }]
    }

    output: #Graph & {
        violations: _violations
        // ... nodes
    }
}
```

### Severity Levels

- **Error**: Blocks deployment
- **Warning**: Allows deployment but logged

## Testing Platform Modules

### Local Testing with CUE CLI

Test your module locally:

```bash
# Create test input
cat > test-input.cue << 'EOF'
package myplatform

testInput: #MyPlatformInput & {
    metadata: {
        name:      "test-app"
        namespace: "default"
    }
    spec: {
        image:    "nginx:latest"
        port:     8080
        replicas: 2
    }
}

testOutput: (#Render & {input: testInput}).output
EOF

# Evaluate
cue eval ./myplatform/ test-input.cue -e testOutput

# Export as YAML
cue export ./myplatform/ test-input.cue -e testOutput --out yaml
```

### Validation Testing

```bash
# Test invalid input (should fail)
cat > invalid-input.cue << 'EOF'
package myplatform

invalidInput: #MyPlatformInput & {
    metadata: { name: "", namespace: "default" }  # Empty name!
    spec: { image: "nginx", port: 70000 }         # Invalid port!
}
EOF

cue eval ./myplatform/ invalid-input.cue
# Should show validation errors
```

### Integration Testing

1. Create a ConfigMap with your module:

```bash
kubectl create configmap myplatform \
  --from-file=schema.cue \
  --from-file=render.cue
```

2. Create a Transform using it:

```yaml
apiVersion: platform.platform.example.com/v1alpha1
kind: Transform
metadata:
  name: test
spec:
  cueRef:
    type: configmap
    ref: myplatform
  input:
    image: nginx:latest
    port: 80
```

3. Verify the ResourceGraph:

```bash
kubectl get resourcegraph -l pequod.io/transform=test -o yaml
```

## Versioning and Distribution

### OCI Registry (Recommended)

Package and push to an OCI registry:

```bash
# Package as OCI artifact
cue mod init myplatform
cue mod tidy
cue mod publish ghcr.io/myorg/platforms/myplatform:v1.0.0

# Or use oras for custom packaging
tar -czf myplatform.tar.gz *.cue
oras push ghcr.io/myorg/platforms/myplatform:v1.0.0 \
  myplatform.tar.gz:application/vnd.cue.module.v1+tar+gzip
```

Reference in Transform:

```yaml
spec:
  cueRef:
    type: oci
    ref: ghcr.io/myorg/platforms/myplatform:v1.0.0
```

### Git Repository

Structure your repository:

```
platforms/
├── webservice/
│   ├── schema.cue
│   └── render.cue
├── database/
│   ├── schema.cue
│   └── render.cue
└── queue/
    ├── schema.cue
    └── render.cue
```

Reference in Transform:

```yaml
spec:
  cueRef:
    type: git
    ref: https://github.com/myorg/platforms.git?ref=v1.0.0&path=webservice
```

### Versioning Best Practices

1. **Semantic Versioning**: Use semver (v1.0.0, v1.1.0, v2.0.0)

2. **Breaking Changes**: Major version bump when:
   - Removing required fields
   - Changing field types
   - Changing resource structure

3. **Non-Breaking Changes**: Minor version bump when:
   - Adding optional fields
   - Adding new nodes
   - Relaxing constraints

4. **Patch Changes**: Patch version bump when:
   - Bug fixes
   - Documentation updates
   - Internal refactoring

### Testing Before Release

```bash
# Run validation
cue vet ./myplatform/

# Check formatting
cue fmt ./myplatform/

# Run all tests
cue eval ./myplatform/...
```

## Example: Complete Database Platform

Here's a complete example of a database platform module:

```cue
// schema.cue
package database

#DatabaseSpec: {
    engine:   "postgres" | "mysql"
    version:  string | *"15"
    storage:  string | *"10Gi"
    replicas: int & >=1 & <=5 | *1

    backup?: {
        enabled:   bool | *true
        schedule:  string | *"0 2 * * *"
        retention: int | *7
    }
}

#DatabaseInput: {
    metadata: { name: string, namespace: string }
    spec: #DatabaseSpec
}
```

```cue
// render.cue
package database

#Render: {
    input: #DatabaseInput

    output: #Graph & {
        metadata: {
            name:        "\(input.metadata.name)-graph"
            platformRef: "database"
        }

        nodes: [
            // PVC for data
            {
                id: "pvc"
                object: {
                    apiVersion: "v1"
                    kind:       "PersistentVolumeClaim"
                    metadata: {
                        name:      "\(input.metadata.name)-data"
                        namespace: input.metadata.namespace
                    }
                    spec: {
                        accessModes: ["ReadWriteOnce"]
                        resources: requests: storage: input.spec.storage
                    }
                }
                applyPolicy: { mode: "Apply" }
                dependsOn: []
                readyWhen: [{ type: "Exists" }]
            },

            // Secret for credentials
            {
                id: "secret"
                object: {
                    apiVersion: "v1"
                    kind:       "Secret"
                    metadata: {
                        name:      "\(input.metadata.name)-credentials"
                        namespace: input.metadata.namespace
                    }
                    stringData: {
                        username: "admin"
                        // In practice, use external secret management
                        password: "changeme"
                    }
                }
                applyPolicy: { mode: "Create" }  // Don't overwrite existing
                dependsOn: []
                readyWhen: [{ type: "Exists" }]
            },

            // StatefulSet
            {
                id: "statefulset"
                object: {
                    apiVersion: "apps/v1"
                    kind:       "StatefulSet"
                    metadata: {
                        name:      input.metadata.name
                        namespace: input.metadata.namespace
                    }
                    spec: {
                        replicas:    input.spec.replicas
                        serviceName: input.metadata.name
                        selector: matchLabels: app: input.metadata.name
                        template: {
                            metadata: labels: app: input.metadata.name
                            spec: {
                                containers: [{
                                    name:  input.spec.engine
                                    image: "\(input.spec.engine):\(input.spec.version)"
                                    volumeMounts: [{
                                        name:      "data"
                                        mountPath: "/var/lib/\(input.spec.engine)"
                                    }]
                                    envFrom: [{
                                        secretRef: name: "\(input.metadata.name)-credentials"
                                    }]
                                }]
                            }
                        }
                        volumeClaimTemplates: [{
                            metadata: name: "data"
                            spec: {
                                accessModes: ["ReadWriteOnce"]
                                resources: requests: storage: input.spec.storage
                            }
                        }]
                    }
                }
                applyPolicy: { mode: "Apply" }
                dependsOn: ["pvc", "secret"]
                readyWhen: [{
                    type:            "ConditionMatch"
                    conditionType:   "Ready"
                    conditionStatus: "True"
                }]
            },

            // Service
            {
                id: "service"
                object: {
                    apiVersion: "v1"
                    kind:       "Service"
                    metadata: {
                        name:      input.metadata.name
                        namespace: input.metadata.namespace
                    }
                    spec: {
                        selector: app: input.metadata.name
                        ports: [{
                            port: {
                                if input.spec.engine == "postgres" { 5432 }
                                if input.spec.engine == "mysql" { 3306 }
                            }
                        }]
                        clusterIP: "None"  // Headless for StatefulSet
                    }
                }
                applyPolicy: { mode: "Apply" }
                dependsOn: ["statefulset"]
                readyWhen: [{ type: "Exists" }]
            },
        ]

        violations: []
    }
}
```

## Getting Help

- [User Guide](user-guide.md) - For users of platform modules
- [Operations Guide](operations.md) - For deploying and monitoring Pequod
- [CUE Documentation](https://cuelang.org/docs/) - CUE language reference
