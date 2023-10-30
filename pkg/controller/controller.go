package controller

import (
	"context"
	"fmt"

	"github.com/buttahtoast/svc-ingress-propagator/pkg/propagation"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// IngressController should implement the Reconciler interface
var _ reconcile.Reconciler = &PropagationController{}

type PropagationController struct {
	logger           logr.Logger
	kubeClient       client.Client
	targetKubeClient client.Client
	options          PropagationControllerOptions
}

func NewPropagationController(logger logr.Logger, kubeClient client.Client, targetKubeClient client.Client, opts PropagationControllerOptions) *PropagationController {
	return &PropagationController{logger: logger, kubeClient: kubeClient, targetKubeClient: targetKubeClient, options: opts}
}

func (i *PropagationController) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	origin := networkingv1.Ingress{}
	err := i.kubeClient.Get(ctx, request.NamespacedName, &origin)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "fetch ingress %s", request.NamespacedName)
	}

	controlled, err := i.isControlledByThisController(ctx, origin)
	if err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, errors.Wrapf(err, "check if ingress %s is controlled by this controller", request.NamespacedName)
	}

	if !controlled {
		i.logger.V(1).Info("ingress is NOT controlled by this controller",
			"ingress", request.NamespacedName,
			"controlled-ingress-class", i.options.IngressClassName,
			"controlled-controller-class", i.options.ControllerClassName,
		)
		return reconcile.Result{
			Requeue: false,
		}, nil
	}

	i.logger.V(1).Info("ingress is controlled by this controller",
		"ingress", request.NamespacedName,
		"controlled-ingress-class", i.options.IngressClassName,
		"controlled-controller-class", i.options.ControllerClassName,
	)

	i.logger.Info("update propagations", "triggered-by", request.NamespacedName)

	err = i.attachFinalizer(ctx, *(origin.DeepCopy()))
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "attach finalizer to ingress %s", request.NamespacedName)
	}

	ingresses, err := i.listControlledIngresses(ctx)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "list controlled ingresses")
	}

	var managedPropagations []propagation.Propagation
	for _, ingress := range ingresses {
		propagations, err := FromIngressToPropagation(ctx, i.logger, i.kubeClient, ingress, i.options.TargetIngressClassName, i.options.Identifier, i.options.TargetNamespace)
		if err != nil {
			i.logger.Info("extract propagations from ingress, skipped", "triggered-by", request.NamespacedName, "ingress", fmt.Sprintf("%s/%s", ingress.Namespace, ingress.Name), "error", err)
		}
		managedPropagations = append(managedPropagations, propagations)
	}
	i.logger.V(3).Info("all propagations", "propagations", managedPropagations)

	err = i.TargetPropagations(ctx, managedPropagations)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "put propagations")
	}

	if origin.DeletionTimestamp != nil {
		err = i.cleanFinalizer(ctx, origin)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "clean finalizer from ingress %s", request.NamespacedName)
		}
	}

	i.logger.V(3).Info("reconcile completed", "triggered-by", request.NamespacedName)
	return reconcile.Result{}, nil
}

func (i *PropagationController) isControlledByThisController(ctx context.Context, target networkingv1.Ingress) (bool, error) {
	if i.options.IngressClassName == target.GetAnnotations()[WellKnownIngressAnnotation] {
		return true, nil
	}

	if target.Spec.IngressClassName == nil {
		return false, nil
	}

	controlledIngressClasses, err := i.listControlledIngressClasses(ctx)
	if err != nil {
		return false, errors.Wrapf(err, "fetch controlled ingress classes with controller name %s", i.options.ControllerClassName)
	}

	var controlledIngressClassNames []string
	for _, controlledIngressClass := range controlledIngressClasses {
		controlledIngressClassNames = append(controlledIngressClassNames, controlledIngressClass.Name)
	}

	if stringSliceContains(controlledIngressClassNames, *target.Spec.IngressClassName) {
		return true, nil
	}

	return false, nil
}

func (i *PropagationController) listControlledIngressClasses(ctx context.Context) ([]networkingv1.IngressClass, error) {
	list := networkingv1.IngressClassList{}
	err := i.kubeClient.List(ctx, &list)
	if err != nil {
		return nil, errors.Wrap(err, "list ingress classes")
	}
	return list.Items, nil
}

func (i *PropagationController) listControlledIngresses(ctx context.Context) ([]networkingv1.Ingress, error) {
	controlledIngressClasses, err := i.listControlledIngressClasses(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "fetch controlled ingress classes with controller name %s", i.options.ControllerClassName)
	}

	var controlledIngressClassNames []string
	for _, controlledIngressClass := range controlledIngressClasses {
		controlledIngressClassNames = append(controlledIngressClassNames, controlledIngressClass.Name)
	}

	var result []networkingv1.Ingress
	list := networkingv1.IngressList{}
	err = i.kubeClient.List(ctx, &list)
	if err != nil {
		return nil, errors.Wrap(err, "list ingresses")
	}

	for _, ingress := range list.Items {
		func() {
			if i.options.IngressClassName == ingress.GetAnnotations()[WellKnownIngressAnnotation] {
				result = append(result, ingress)
				return
			}

			if ingress.Spec.IngressClassName == nil {
				return
			}

			if stringSliceContains(controlledIngressClassNames, *ingress.Spec.IngressClassName) {
				result = append(result, ingress)
				return
			}
		}()
	}

	return result, nil
}
