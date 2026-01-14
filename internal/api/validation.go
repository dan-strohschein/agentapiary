package api

import (
	"fmt"
	"regexp"
)

// namespaceRegex validates DNS-1123 subdomain format: [a-z0-9]([-a-z0-9]*[a-z0-9])?
var namespaceRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validateNamespace validates a namespace name.
func validateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	
	// DNS-1123 subdomain format: [a-z0-9]([-a-z0-9]*[a-z0-9])?
	// Max length 253 characters
	if len(namespace) > 253 {
		return fmt.Errorf("namespace must be at most 253 characters")
	}
	
	if !namespaceRegex.MatchString(namespace) {
		return fmt.Errorf("namespace must match DNS-1123 subdomain format: [a-z0-9]([-a-z0-9]*[a-z0-9])?")
	}
	
	return nil
}

// validateResourceNamespace validates that a resource's namespace matches the URL namespace.
func validateResourceNamespace(resourceNamespace, urlNamespace string) error {
	if resourceNamespace == "" {
		return fmt.Errorf("resource namespace is required")
	}
	
	if resourceNamespace != urlNamespace {
		return fmt.Errorf("resource namespace (%s) does not match URL namespace (%s)", resourceNamespace, urlNamespace)
	}
	
	return validateNamespace(resourceNamespace)
}
