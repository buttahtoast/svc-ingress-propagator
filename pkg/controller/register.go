package controller

import (
	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type PropagationControllerOptions struct {
	Identifier             string
	IngressClassName       string
	ControllerClassName    string
	TargetIngressClassName string
	TargetNamespace        string
}

func RegisterPropagationController(logger logr.Logger, mgr manager.Manager, targetKubeClient client.Client, options PropagationControllerOptions) error {

	controller := NewPropagationController(logger.WithName("ingress-propagator"), mgr.GetClient(), targetKubeClient, options)
	err := builder.
		ControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(controller)

	if err != nil {
		logger.WithName("register-controller").Error(err, "could not register propagation controller")
		return err
	}

	if err != nil {
		logger.WithName("register-controller").Error(err, "could not register propagation controller")
		return err
	}

	return nil
}
