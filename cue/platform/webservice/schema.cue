package webservice

// #Input is the schema used for CRD generation
// Aliased from #WebServiceSpec for compatibility with the schema extractor
#Input: #WebServiceSpec

// #WebServiceSpec defines the schema for WebService input
// This matches the Go type in api/v1alpha1/webservice_types.go
#WebServiceSpec: {
	// image is the container image to deploy (required)
	image: string & !=""

	// replicas is the number of replicas to deploy (optional)
	// If not specified, the default from policy will be used
	replicas?: int & >=0

	// port is the service port to expose (required)
	port: int & >=1 & <=65535

	// platformRef references the platform module to use (optional)
	// Defaults to "embedded"
	platformRef?: string

	// envFrom specifies sources to populate environment variables (optional)
	envFrom?: [...#EnvFromSource]
}

// #EnvFromSource represents a source for environment variables
#EnvFromSource: {
	// secretRef references a Secret
	secretRef?: #SecretReference

	// configMapRef references a ConfigMap
	configMapRef?: #ConfigMapReference
}

// #SecretReference contains enough information to locate a Secret
#SecretReference: {
	// name of the Secret
	name: string & !=""
}

// #ConfigMapReference contains enough information to locate a ConfigMap
#ConfigMapReference: {
	// name of the ConfigMap
	name: string & !=""
}

// #WebServiceInput is the complete input structure
#WebServiceInput: {
	// metadata contains the WebService resource metadata
	metadata: {
		name:      string
		namespace: string
	}

	// spec is the WebService specification
	spec: #WebServiceSpec
}

