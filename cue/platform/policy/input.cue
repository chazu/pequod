package policy

// #InputPolicy defines validation rules for WebService inputs
#InputPolicy: {
	// Image registry policy
	imageRegistry: {
		// allowedRegistries is a list of allowed container registries
		allowedRegistries: [...string]
		
		// If set, images must come from one of the allowed registries
		enforceRegistry: bool | *false
	}

	// Resource limits policy
	resourceLimits: {
		// Maximum number of replicas allowed
		maxReplicas: int | *10
		
		// Minimum number of replicas allowed
		minReplicas: int | *1
	}

	// Port policy
	portPolicy: {
		// Allowed port ranges
		allowedPorts: [...{min: int, max: int}]
		
		// If set, ports must be in allowed ranges
		enforcePorts: bool | *false
	}
}

// DefaultPolicy provides sensible defaults
DefaultPolicy: #InputPolicy & {
	imageRegistry: {
		allowedRegistries: [
			"docker.io",
			"gcr.io",
			"ghcr.io",
			"public.ecr.aws",
		]
		enforceRegistry: false
	}
	
	resourceLimits: {
		maxReplicas: 10
		minReplicas: 1
	}
	
	portPolicy: {
		allowedPorts: [
			{min: 80, max: 80},
			{min: 443, max: 443},
			{min: 8000, max: 9000},
		]
		enforcePorts: false
	}
}

// ValidateInput checks if input conforms to policy
// This is a simplified version - full validation will be done in Go code
#ValidateInput: {
	input: {
		spec: {
			image:     string
			replicas?: int
			port:      int
		}
	}

	policy: #InputPolicy

	// Violations will be populated if validation fails
	// The actual validation logic will be implemented in Go
	violations: [...{
		path:     string
		message:  string
		severity: "Error" | "Warning"
	}] | *[]
}

