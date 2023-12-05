package propagation

import (
	v1 "k8s.io/api/core/v1"                 // For Service and Endpoints
	networkingv1 "k8s.io/api/networking/v1" // For Ingress
)

// Exposure is the minimal information for exposing a service.
type Propagation struct {
	// Propagation Name
	Name string

	PropagatedName string

	// State if the propagation is deleted on the child cluster
	IsDeleted bool

	// Origin Ingress
	Origin networkingv1.Ingress

	// The ingress object associated with the propagation.
	Ingress networkingv1.Ingress

	// The list of services associated with the propagation.
	Services []v1.Service

	// The list of endpoints associated with the propagation.
	Endpoints []v1.Endpoints
}
