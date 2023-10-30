package main

import (
	"context"
	"log"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/oliverbaehler/svc-ingress-propagator/pkg/controller"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type rootCmdFlags struct {
	controllerClass string
	logger          logr.Logger
	// for annotation on Ingress
	ingressClass string
	// for identifying objects on parent cluster
	identifier string
	// Kubeconfig for parent cluster
	kubeconfig string
	// Binary log level
	logLevel int
	// Ingress class on loadbalancer cluster
	targetIngressClass string
	targetNamespace    string
	targetKubeconfig   string
}

func main() {
	var rootLogger = stdr.NewWithOptions(log.New(os.Stderr, "", log.LstdFlags), stdr.Options{LogCaller: stdr.All})

	options := rootCmdFlags{
		logger:             rootLogger.WithName("main"),
		ingressClass:       "propagator",
		targetIngressClass: "propagator",
		targetNamespace:    "propagator",
		controllerClass:    "buttah.cloud/svc-ingress-propagator",
		logLevel:           0,
	}

	crlog.SetLogger(rootLogger.WithName("controller-runtime"))

	rootCommand := cobra.Command{
		Use: "tunnel-controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			stdr.SetVerbosity(options.logLevel)
			logger := options.logger
			logger.Info("logging verbosity", "level", options.logLevel)

			cfg := config.GetConfigOrDie()

			// Load the kubeconfig from the provided file path
			target, err := clientcmd.BuildConfigFromFlags("", options.targetKubeconfig)
			if err != nil {
				logger.Error(err, "unable to load target kubeconfig")
				os.Exit(1)
			}
			targetClient, err := client.New(target, client.Options{})
			if err != nil {
				logger.Error(err, "unable to set up target client")
				os.Exit(1)
			}

			mgr, err := manager.New(cfg, manager.Options{})
			if err != nil {
				logger.Error(err, "unable to set up manager")
				os.Exit(1)
			}

			logger.Info("propagation controller start serving")
			err = controller.RegisterPropagationController(logger, mgr,
				targetClient,
				controller.PropagationControllerOptions{
					Identifier:             options.identifier,
					IngressClassName:       options.ingressClass,
					TargetIngressClassName: options.targetIngressClass,
					ControllerClassName:    options.controllerClass,
				})
			if err != nil {
				return err
			}

			// controller-runtime manager would graceful shutdown with signal by itself, no need to provide context
			return mgr.Start(context.Background())
		},
	}

	rootCommand.PersistentFlags().StringVar(&options.ingressClass, "ingress-class", options.ingressClass, "ingress class name")
	rootCommand.PersistentFlags().StringVar(&options.controllerClass, "controller-class", options.controllerClass, "controller class name")
	rootCommand.PersistentFlags().IntVarP(&options.logLevel, "log-level", "v", options.logLevel, "numeric log level")
	rootCommand.PersistentFlags().StringVar(&options.targetIngressClass, "target-ingress-class", options.targetIngressClass, "Ingress Class on target cluster")
	rootCommand.PersistentFlags().StringVar(&options.identifier, "identifier", options.identifier, "propagator identifier, if multiple propagators sync to the same target namespace, this should be different for each")
	rootCommand.PersistentFlags().StringVar(&options.targetNamespace, "target-namespace", options.targetNamespace, "namespace on target cluster, where manifests are synced to")
	rootCommand.PersistentFlags().StringVar(&options.targetKubeconfig, "target-kubeconfig", options.targetKubeconfig, "namespace on target cluster, where manifests are synced to")

	err := rootCommand.Execute()
	if err != nil {
		panic(err)
	}
}
