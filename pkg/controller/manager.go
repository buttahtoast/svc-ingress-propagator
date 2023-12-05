package controller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const IngressControllerFinalizer = "svc-ingress-propagator.buttah.cloud/propagated-ingress"

// IngressController should implement the Reconciler interface
var _ reconcile.Reconciler = &PropagationController{}

type PropagationController struct {
	Client       client.Client
	TargetClient client.Client
	Log          logr.Logger
	Recorder     record.EventRecorder
	Options      PropagationControllerOptions
}

type PropagationControllerOptions struct {
	Identifier             string
	IngressClassName       string
	ControllerClassName    string
	TargetIngressClassName string
	TargetNamespace        string
	TargetIssuerName       string
	TargetIssuerNamespaced bool
	TLSrespect             bool
}

func (i *PropagationController) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1.Ingress{}).
		Complete(i)
}

func (i *PropagationController) Reconcile(ctx context.Context, request ctrl.Request) (res reconcile.Result, err error) {
	i.Log.V(3).Info("Reconciling",
		"ingress", request.NamespacedName,
	)
	origin := networkingv1.Ingress{}
	if err = i.Client.Get(ctx, request.NamespacedName, &origin); err != nil {
		if apierrors.IsNotFound(err) {
			i.Log.Error(nil, "Request object not found, could have been deleted after reconcile request")

			return reconcile.Result{}, nil
		}

		i.Log.Error(err, "Error reading the object")

		return
	}

	controlled, err := i.isControlledByThisController(ctx, origin)
	if err != nil && !apierrors.IsNotFound(err) {
		i.Log.V(3).Error(err, "check if ingress %s is controlled by this controller", request.NamespacedName)
		return reconcile.Result{
			RequeueAfter: time.Second * 60,
		}, nil
	}

	if !controlled {
		i.Log.V(5).Info("ingress is NOT controlled by this controller",
			"ingress", request.NamespacedName,
			"controlled-ingress-class", i.Options.IngressClassName,
			"controlled-controller-class", i.Options.ControllerClassName,
		)
		return reconcile.Result{
			Requeue: false,
		}, nil
	}

	i.Log.V(5).Info("ingress is controlled by this controller",
		"ingress", request.NamespacedName,
		"controlled-ingress-class", i.Options.IngressClassName,
		"controlled-controller-class", i.Options.ControllerClassName,
	)

	i.Log.V(5).Info("update propagations", "triggered-by", request.NamespacedName)

	if origin.DeletionTimestamp == nil {
		controllerutil.AddFinalizer(&origin, IngressControllerFinalizer)
	} else {
		if !controllerutil.ContainsFinalizer(&origin, IngressControllerFinalizer) {
			i.Log.V(1).Info("ingress is being deleted and already finillized by this controller",
				"ingress", request.NamespacedName,
				"controlled-ingress-class", i.Options.IngressClassName,
				"controlled-controller-class", i.Options.ControllerClassName,
			)

			return
		}
	}

	propagation, err := i.FromIngressToPropagation(ctx, i.Log, i.Client, origin)
	if err != nil {
		i.Recorder.Eventf(&origin, corev1.EventTypeWarning, "PropagationFailed", "failed to extract propagations from ingress: %s", err.Error())

		return reconcile.Result{
			RequeueAfter: time.Second * 60,
		}, nil
	}

	i.Log.V(5).Info("all propagations", "propagations", propagation.Ingress)
	err = i.TargetPropagation(ctx, propagation)
	if err != nil {
		i.Log.Error(err, "put propagations")

		return reconcile.Result{
			RequeueAfter: time.Second * 60,
		}, nil
	}

	if origin.DeletionTimestamp != nil {
		controllerutil.RemoveFinalizer(&origin, IngressControllerFinalizer)
	}

	i.Log.V(3).Info("Reconcile completed")
	return
}
