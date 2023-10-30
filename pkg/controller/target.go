package controller

import (
	"context"

	"github.com/oliverbaehler/svc-ingress-propagator/pkg/propagation"

	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
)

func (i *PropagationController) TargetPropagations(ctx context.Context, propagations []propagation.Propagation) error {
	err := i.propagateIngress(ctx, propagations)
	if err != nil {
		return errors.Wrap(err, "ingress propagation")
	}

	err = i.propagateEndpoint(ctx, propagations)
	if err != nil {
		return errors.Wrap(err, "endpoint propagation")
	}

	err = i.propagateService(ctx, propagations)
	if err != nil {
		return errors.Wrap(err, "service propagation")
	}

	return nil
}

func (i *PropagationController) propagateIngress(ctx context.Context, propagations []propagation.Propagation) error {
	for _, prop := range propagations {
		if prop.IsDeleted {
			// Delete the ingress
			err := i.targetKubeClient.Delete(ctx, &prop.Ingress)
			if err != nil {
				return errors.Wrapf(err, "failed to delete ingress %s", prop.Ingress.Name)
			}
		} else {
			// Try to update the ingress
			err := i.targetKubeClient.Update(ctx, &prop.Ingress)
			if err != nil {
				// If error is because the resource doesn't exist, then create it
				if k8serrors.IsNotFound(err) {
					err = i.targetKubeClient.Create(ctx, &prop.Ingress)
					if err != nil {
						return errors.Wrapf(err, "failed to create ingress %s", prop.Ingress.Name)
					}
				} else {
					return errors.Wrapf(err, "failed to update ingress %s", prop.Ingress.Name)
				}
			}
		}
	}
	return nil
}

func (i *PropagationController) propagateEndpoint(ctx context.Context, propagations []propagation.Propagation) error {
	for _, prop := range propagations {
		if prop.IsDeleted {
			// Delete all releated endpoints
			selector := labels.Set{
				LabelManaged:    i.options.Identifier,
				LabelPropagator: prop.Name,
			}
			listOptions := client.ListOptions{LabelSelector: labels.SelectorFromSet(selector), Namespace: i.options.TargetNamespace}

			var endpointsList v1.EndpointsList
			err := i.targetKubeClient.List(ctx, &endpointsList, &listOptions)
			if err != nil {
				return errors.Wrap(err, "failed to list endpoints with label selector")
			}

			for _, endpoint := range endpointsList.Items {
				err = i.targetKubeClient.Delete(ctx, &endpoint)
				if err != nil {
					return errors.Wrapf(err, "failed to delete endpoint %s in namespace %s", endpoint.Name, endpoint.Namespace)
				}
			}
		} else {
			for _, endpoint := range prop.Endpoints {
				// Try to update the endpoint
				err := i.targetKubeClient.Update(ctx, &endpoint)
				if err != nil {
					// If error is because the resource doesn't exist, then create it
					if k8serrors.IsNotFound(err) {
						err = i.targetKubeClient.Create(ctx, &endpoint)
						if err != nil {
							return errors.Wrapf(err, "failed to create endpoint %s in namespace %s", endpoint.Name, endpoint.Namespace)
						}
					} else {
						return errors.Wrapf(err, "failed to update endpoint %s in namespace %s", endpoint.Name, endpoint.Namespace)
					}
				}
			}
		}
	}
	return nil
}

func (i *PropagationController) propagateService(ctx context.Context, propagations []propagation.Propagation) error {
	for _, prop := range propagations {
		if prop.IsDeleted {
			selector := labels.Set{
				LabelManaged:    i.options.Identifier,
				LabelPropagator: prop.Name,
			}
			listOptions := client.ListOptions{LabelSelector: labels.SelectorFromSet(selector), Namespace: i.options.TargetNamespace}

			// List services with the label selector
			var servicesList v1.ServiceList
			err := i.targetKubeClient.List(ctx, &servicesList, &listOptions)
			if err != nil {
				return errors.Wrap(err, "failed to list services with label selector")
			}

			// Delete each service from the list
			for _, service := range servicesList.Items {
				err = i.targetKubeClient.Delete(ctx, &service)
				if err != nil {
					return errors.Wrapf(err, "failed to delete service %s in namespace %s", service.Name, service.Namespace)
				}
			}
		} else {
			for _, service := range prop.Services {
				// Try to update the service
				err := i.targetKubeClient.Update(ctx, &service)
				if err != nil {
					// If error is because the resource doesn't exist, then create it
					if k8serrors.IsNotFound(err) {
						err = i.targetKubeClient.Create(ctx, &service)
						if err != nil {
							return errors.Wrapf(err, "failed to create service %s in namespace %s", service.Name, service.Namespace)
						}
					} else {
						return errors.Wrapf(err, "failed to update service %s in namespace %s", service.Name, service.Namespace)
					}
				}
			}
		}
	}
	return nil
}
