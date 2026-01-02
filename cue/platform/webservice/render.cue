package webservice

// #Graph is the output graph structure
#Graph: {
	metadata: {
		name:        string
		version:     "v1alpha1"
		platformRef: string
	}
	nodes:      [...#Node]
	violations: [...#Violation]
}

// #Node represents a single resource in the graph
#Node: {
	id:          string
	object:      _
	applyPolicy: #ApplyPolicy
	dependsOn: [...string]
	readyWhen: [...#ReadinessPredicate]
}

// #ApplyPolicy defines how a resource should be applied
#ApplyPolicy: {
	mode:           *"Apply" | "Create" | "Adopt"
	conflictPolicy: *"Error" | "Force"
	fieldManager?:  string
}

// #ReadinessPredicate defines when a resource is ready
#ReadinessPredicate: {
	type: "ConditionMatch" | "DeploymentAvailable" | "Exists"
	// For ConditionMatch
	conditionType?:   string
	conditionStatus?: string
}

// #Violation represents a policy violation
#Violation: {
	path:     string
	message:  string
	severity: *"Error" | "Warning"
}

// Render takes a WebServiceInput and produces a Graph
#Render: {
	input: #WebServiceInput

	// Output graph
	output: #Graph & {
		metadata: {
			name: "\(input.metadata.name)-graph"
			// Use input platformRef if provided, otherwise default to "embedded"
			// Note: Use if/else pattern to avoid CUE disjunction default behavior
			if input.spec.platformRef != _|_ {
				platformRef: input.spec.platformRef
			}
			if input.spec.platformRef == _|_ {
				platformRef: "embedded"
			}
		}

		nodes: [
			// Deployment node
			{
				id: "deployment"
				object: {
					apiVersion: "apps/v1"
					kind:       "Deployment"
					metadata: {
						name:      input.metadata.name
						namespace: input.metadata.namespace
						labels: {
							"app.kubernetes.io/name":       input.metadata.name
							"app.kubernetes.io/managed-by": "pequod"
						}
					}
					spec: {
						replicas: *1 | int
						if input.spec.replicas != _|_ {
							replicas: input.spec.replicas
						}
						selector: matchLabels: {
							"app.kubernetes.io/name": input.metadata.name
						}
						template: {
							metadata: labels: {
								"app.kubernetes.io/name": input.metadata.name
							}
							spec: containers: [{
								name:  input.metadata.name
								image: input.spec.image
								ports: [{
									containerPort: input.spec.port
									name:          "http"
								}]

								// Add envFrom if specified
								if input.spec.envFrom != _|_ {
									envFrom: input.spec.envFrom
								}
							}]
						}
					}
				}
				applyPolicy: {
					mode:           "Apply"
					conflictPolicy: "Error"
				}
				dependsOn: []
				readyWhen: [{
					type: "DeploymentAvailable"
				}]
			},

			// Service node
			{
				id: "service"
				object: {
					apiVersion: "v1"
					kind:       "Service"
					metadata: {
						name:      input.metadata.name
						namespace: input.metadata.namespace
						labels: {
							"app.kubernetes.io/name":       input.metadata.name
							"app.kubernetes.io/managed-by": "pequod"
						}
					}
					spec: {
						selector: {
							"app.kubernetes.io/name": input.metadata.name
						}
						ports: [{
							port:       input.spec.port
							targetPort: "http"
							protocol:   "TCP"
							name:       "http"
						}]
						type: "ClusterIP"
					}
				}
				applyPolicy: {
					mode:           "Apply"
					conflictPolicy: "Error"
				}
				dependsOn: ["deployment"]
				readyWhen: [{
					type: "Exists"
				}]
			},
		]

		violations: []
	}
}

