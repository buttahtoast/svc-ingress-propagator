package controller

import (
	"context"

	"github.com/pkg/errors"
	networkingv1 "k8s.io/api/networking/v1"
)

const IngressControllerFinalizer = "svc-ingress-propagator.buttah.cloud/propagated-ingress"

func (i *PropagationController) attachFinalizer(ctx context.Context, ingress networkingv1.Ingress) error {
	if stringSliceContains(ingress.Finalizers, IngressControllerFinalizer) {
		return nil
	}
	ingress.Finalizers = append(ingress.Finalizers, IngressControllerFinalizer)
	err := i.kubeClient.Update(ctx, &ingress)
	if err != nil {
		return errors.Wrapf(err, "attach finalizer for %s/%s", ingress.Namespace, ingress.Name)
	}
	return nil
}

func (i *PropagationController) cleanFinalizer(ctx context.Context, ingress networkingv1.Ingress) error {
	if !stringSliceContains(ingress.Finalizers, IngressControllerFinalizer) {
		return nil
	}
	ingress.Finalizers = removeStringFromSlice(ingress.Finalizers, IngressControllerFinalizer)
	err := i.kubeClient.Update(ctx, &ingress)
	if err != nil {
		return errors.Wrapf(err, "clean finalizer for %s/%s", ingress.Namespace, ingress.Name)
	}
	return nil
}
