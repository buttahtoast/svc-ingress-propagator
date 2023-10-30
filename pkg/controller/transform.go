package controller

import (
	"context"
	"fmt"

	"github.com/oliverbaehler/svc-ingress-propagator/pkg/propagation"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func FromIngressToPropagation(ctx context.Context, logger logr.Logger, kubeClient client.Client, ingress networkingv1.Ingress, ingressClass string, identifier string, namespace string) (propagation.Propagation, error) {
	result := propagation.Propagation{}
	ing := networkingv1.Ingress{}
	result.IsDeleted = false

	if ingress.DeletionTimestamp != nil {
		result.IsDeleted = true
	}

	// Assign Name
	result.Name = ingress.Name
	ing.Name = result.Name
	ing.SetNamespace(namespace)

	if ingress.Spec.TLS != nil {
		ing.Spec.TLS = ingress.Spec.TLS
	}

	// Assign Annotations
	if ingress.Annotations != nil {
		ing.Annotations = ingress.Annotations
	}
	// Assign Labels
	if ingress.Labels != nil {
		ing.Labels = ingress.Labels
	} else {
		ing.Labels = make(map[string]string)
	}
	ing.Labels[LabelManaged] = identifier

	ing.Spec.IngressClassName = &ingressClass

	// Store relevant Services
	var services []v1.Service

	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" {
			return result, errors.Errorf("host in ingress %s/%s is empty", ingress.GetNamespace(), ingress.GetName())
		}

		for _, path := range rule.HTTP.Paths {

			namespacedName := types.NamespacedName{
				Namespace: ingress.GetNamespace(),
				Name:      path.Backend.Service.Name,
			}
			service := v1.Service{}
			err := kubeClient.Get(ctx, namespacedName, &service)
			if err != nil {
				return result, errors.Wrapf(err, "fetch service %s", namespacedName)
			}

			if service.Status.LoadBalancer.Ingress == nil {
				return result, errors.Errorf("service %s has no loadbalancer ip", namespacedName)
			}

			if !containsService(services, path.Backend.Service.Name) {
				services = append(services, service)
			}

			var port int32
			if path.Backend.Service.Port.Name != "" {
				ok, extractedPort := getPortWithName(service.Spec.Ports, path.Backend.Service.Port.Name)
				if !ok {
					return result, errors.Errorf("service %s has no port named %s", namespacedName, path.Backend.Service.Port.Name)
				}
				port = extractedPort
			} else {
				port = path.Backend.Service.Port.Number
			}

			path.Backend = networkingv1.IngressBackend{
				Service: &networkingv1.IngressServiceBackend{
					Name: result.Name,
					Port: networkingv1.ServiceBackendPort{
						Number: port,
					},
				},
			}
		}
	}
	ing.Spec.Rules = ingress.Spec.Rules

	result.Ingress = ing

	// Load Services and endpoints
	error := resolveServiceEndpoints(services, &result, identifier, namespace)
	if error != nil {
		return result, errors.Wrapf(error, "failed to resolve service endpoints")
	}

	return result, nil
}

func resolveServiceEndpoints(services []v1.Service, propagation *propagation.Propagation, identifier string, namespace string) error {
	for _, oldService := range services {
		// Unset any Nodeport
		for idx := range oldService.Spec.Ports {
			oldService.Spec.Ports[idx].NodePort = 0
		}

		// Create new service struct
		service := v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      oldService.Name, // Assuming same name, adjust if necessary
				Namespace: namespace,
				Labels: map[string]string{
					LabelManaged:    identifier,
					LabelPropagator: propagation.Name,
				},
			},
			Spec: v1.ServiceSpec{
				Type:  "ClusterIP",
				Ports: oldService.Spec.Ports,
				// Add other necessary fields
			},
		}
		propagation.Services = append(propagation.Services, service)

		// Create endpoint for the service
		endpointSubsets := []v1.EndpointSubset{}

		addresses := []v1.EndpointAddress{}
		for _, ingress := range oldService.Status.LoadBalancer.Ingress {
			if ingress.IP != "" {
				addresses = append(addresses, v1.EndpointAddress{IP: ingress.IP})
			} else if ingress.Hostname != "" {
				addresses = append(addresses, v1.EndpointAddress{Hostname: ingress.Hostname})
			}
		}

		for _, port := range oldService.Spec.Ports {
			endpointSubsets = append(endpointSubsets, v1.EndpointSubset{
				Addresses: addresses,
				Ports: []v1.EndpointPort{
					{
						Name:     port.Name,
						Port:     port.Port,
						Protocol: port.Protocol,
					},
				},
			})
		}

		endpoint := v1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      oldService.Name, // Assuming same name, adjust if necessary
				Namespace: namespace,
				Labels: map[string]string{
					LabelManaged:    identifier,
					LabelPropagator: propagation.Name,
				},
			},
			Subsets: endpointSubsets,
		}
		propagation.Endpoints = append(propagation.Endpoints, endpoint)
	}
	fmt.Printf("HIHO %v\n\n", propagation)
	return nil
}

func containsService(services []v1.Service, serviceName string) bool {
	for _, svc := range services {
		if svc.Name == serviceName {
			return true
		}
	}
	return false
}

func getPortWithName(ports []v1.ServicePort, portName string) (bool, int32) {
	for _, port := range ports {
		if port.Name == portName {
			return true, port.Port
		}
	}
	return false, 0
}
