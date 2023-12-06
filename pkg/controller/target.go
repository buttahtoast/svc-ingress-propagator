package controller

import (
	"context"
	"fmt"

	"github.com/buttahtoast/svc-ingress-propagator/pkg/propagation"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (i *PropagationController) putPropagation(ctx context.Context, prop propagation.Propagation) error {
	// Try to update the ingress
	err := i.TargetClient.Update(ctx, &prop.Ingress)
	if err != nil {
		// If error is because the resource doesn't exist, then create it
		if k8serrors.IsNotFound(err) {
			err = i.TargetClient.Create(ctx, &prop.Ingress)
			if err != nil {
				return fmt.Errorf("failed to create ingress %s: %s", prop.Ingress.Name, err)
			}
		} else {
			return fmt.Errorf("failed to update ingress %s: %s", prop.Ingress.Name, err)
		}
	}
	// Fetch the updated ingress to get the UID
	updatedIngress := &v1.Ingress{}
	err = i.TargetClient.Get(ctx, types.NamespacedName{Name: prop.Ingress.Name, Namespace: prop.Ingress.Namespace}, updatedIngress)
	if err != nil {
		return fmt.Errorf("failed to get ingress on target cluster %s: %s", prop.Ingress.Name, err)
	}

	// OwnerReference using the Ingress UID
	ownerRef := metav1.OwnerReference{
		APIVersion: "networking.k8s.io/v1",
		Kind:       "Ingress",
		Name:       updatedIngress.Name,
		UID:        updatedIngress.ObjectMeta.GetUID(),
	}

	// Update Endpoints
	for _, endpoint := range prop.Endpoints {
		endpoint.OwnerReferences = append(endpoint.OwnerReferences, ownerRef)
		err := i.TargetClient.Update(ctx, &endpoint)
		if err != nil {
			// If error is because the resource doesn't exist, then create it
			if k8serrors.IsNotFound(err) {
				err = i.TargetClient.Create(ctx, &endpoint)
				if err != nil {
					return fmt.Errorf("failed to create endpoint %s in namespace %s: %s", endpoint.Name, endpoint.Namespace, err)
				}
			} else {
				return fmt.Errorf("failed to update endpoint %s in namespace %s: %s", endpoint.Name, endpoint.Namespace, err)
			}
		}
	}
	for _, service := range prop.Services {
		service.OwnerReferences = append(service.OwnerReferences, ownerRef)
		err := i.TargetClient.Update(ctx, &service)
		if err != nil {
			// If error is because the resource doesn't exist, then create it
			if k8serrors.IsNotFound(err) {
				err = i.TargetClient.Create(ctx, &service)
				if err != nil {
					return fmt.Errorf("failed to create service %s in namespace %s: %s", service.Name, service.Namespace, err)
				}
			} else {
				return fmt.Errorf("failed to update service %s in namespace %s: %s", service.Name, service.Namespace, err)
			}
		}
	}

	i.Recorder.Eventf(&prop.Origin, corev1.EventTypeNormal, "IngressPropagated", "Ingress has been propagated")
	return nil
}

func (i *PropagationController) removePropagation(ctx context.Context, prop propagation.Propagation) error {
	err := i.TargetClient.Delete(ctx, &prop.Ingress)
	if !k8serrors.IsNotFound(err) {
		if err != nil {
			return fmt.Errorf("failed to delete ingress %s: %s", prop.Ingress.Name, err)
		}
	}

	i.Recorder.Eventf(&prop.Origin, corev1.EventTypeNormal, "IngressUnpropagated", "Ingress has been removed")
	return nil
}
