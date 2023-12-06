package controller

import (
	"context"
	"fmt"

	"github.com/buttahtoast/svc-ingress-propagator/pkg/propagation"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	hosts    []string
	services []v1.Service
)

func (i *PropagationController) FromIngressToPropagation(ctx context.Context, logger logr.Logger, kubeClient client.Client, ingress networkingv1.Ingress) (propagation.Propagation, error) {
	result := propagation.Propagation{
		Name:           ingress.Name,
		PropagatedName: fmt.Sprintf("%s-%s", i.Options.Identifier, ingress.Name),
		IsDeleted:      false,
		Ingress: networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", i.Options.Identifier, ingress.Name),
				Namespace: i.Options.TargetNamespace,
			},
		},
		Origin: ingress,
	}

	if ingress.DeletionTimestamp != nil {
		result.IsDeleted = true
	} else {

		// Assign Labels
		result.Ingress.Labels = make(map[string]string)
		if ingress.Labels != nil {
			result.Ingress.Labels = ingress.Labels
		}
		result.Ingress.Labels[LabelManaged] = i.Options.Identifier

		// Annotations
		result.Ingress.Annotations = make(map[string]string)
		if ingress.Annotations != nil {
			result.Ingress.Annotations = ingress.Annotations
		}

		result.Ingress.Spec.IngressClassName = &i.Options.TargetIngressClassName

		result.Ingress.Spec.Rules = ingress.Spec.Rules
		for r := range result.Ingress.Spec.Rules {
			rule := &result.Ingress.Spec.Rules[r]
			if rule.Host == "" {
				return result, fmt.Errorf("host in ingress %s/%s is empty", ingress.GetNamespace(), ingress.GetName())
			}

			for p := range rule.HTTP.Paths {
				path := &rule.HTTP.Paths[p]

				namespacedName := types.NamespacedName{
					Namespace: ingress.GetNamespace(),
					Name:      path.Backend.Service.Name,
				}
				service := v1.Service{}
				err := kubeClient.Get(ctx, namespacedName, &service)
				if err != nil {
					return result, fmt.Errorf("fetch service %s: %s", namespacedName, err)
				}

				if service.Status.LoadBalancer.Ingress == nil {
					return result, fmt.Errorf("service %s has no loadbalancer ip", namespacedName)
				}

				var port int32
				if path.Backend.Service.Port.Name != "" {
					ok, extractedPort := getPortWithName(service.Spec.Ports, path.Backend.Service.Port.Name)
					if !ok {
						return result, fmt.Errorf("service %s has no port named %s", namespacedName, path.Backend.Service.Port.Name)
					}
					port = extractedPort
				} else {
					port = path.Backend.Service.Port.Number
				}

				service.ObjectMeta.Name = result.PropagatedName
				if !containsService(services, path.Backend.Service.Name) {
					services = append(services, service)
				}

				path.Backend = networkingv1.IngressBackend{
					Service: &networkingv1.IngressServiceBackend{
						Name: result.PropagatedName,
						Port: networkingv1.ServiceBackendPort{
							Number: port,
						},
					},
				}
			}
			hosts = append(hosts, rule.Host)
		}

		// Add TLS information
		if (i.Options.TLSrespect) && (ingress.Spec.TLS != nil) {
			result.Ingress.Spec.TLS = ingress.Spec.TLS
		}

		if i.Options.TargetIssuerName != "" {
			if i.Options.TargetIssuerNamespaced {
				result.Ingress.Annotations[IssuerNamespacedAnnotation] = i.Options.TargetIssuerName
			} else {
				result.Ingress.Annotations[IssuerClusterAnnotation] = i.Options.TargetIssuerName
			}
			result.Ingress.Spec.TLS = append(result.Ingress.Spec.TLS, networkingv1.IngressTLS{
				Hosts:      hosts,
				SecretName: result.PropagatedName,
			})
		}

		// Load Services and endpoints
		err := resolveServiceEndpoints(services, &result, i.Options.Identifier, i.Options.TargetNamespace)
		if err != nil {
			return result, fmt.Errorf("failed to resolve service endpoints: %s", err)
		}
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
