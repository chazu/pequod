package webservice

// Example test case
example: #Render & {
	input: {
		metadata: {
			name:      "my-app"
			namespace: "default"
		}
		spec: {
			image: "nginx:latest"
			port:  8080
			replicas: 3
		}
	}
}

