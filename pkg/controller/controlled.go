package controller

import (
	"context"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
)

func (i *PropagationController) isControlledByThisController(ctx context.Context, target networkingv1.Ingress) (bool, error) {
	if i.Options.IngressClassName == target.GetAnnotations()[WellKnownIngressAnnotation] {
		return true, nil
	}

	if target.Spec.IngressClassName == nil {
		return false, nil
	}

	controlledIngressClassNames, err := i.listControlledIngressClasses(ctx)
	if err != nil {
		return false, fmt.Errorf("fetch controlled ingress classes with controller name %s", i.Options.ControllerClassName)
	}

	if stringSliceContains(controlledIngressClassNames, *target.Spec.IngressClassName) {
		return true, nil
	}

	return false, nil
}

func (i *PropagationController) listControlledIngressClasses(ctx context.Context) ([]string, error) {
	list := networkingv1.IngressClassList{}
	err := i.Client.List(ctx, &list)
	if err != nil {
		return nil, err
	}

	var controlledNames []string
	for _, ingressClass := range list.Items {
		// Check if the IngressClass is controlled by the specified controller
		if ingressClass.Spec.Controller == i.Options.ControllerClassName {
			controlledNames = append(controlledNames, ingressClass.Name)
		}
	}

	return controlledNames, nil
}
