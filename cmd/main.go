package main

import (
	"fmt"
	"log"
	"os"

	"github.com/buttahtoast/svc-ingress-propagator/pkg/controller"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/spf13/cobra"
	_ "go.uber.org/automaxprocs"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type rootCmdFlags struct {
	controllerClass string
	logger          logr.Logger
	// for annotation on Ingress
	ingressClass string
	// for identifying objects on parent cluster
	identifier string
	// Binary log level
	logLevel int
	// Ingress class on loadbalancer cluster
	targetIngressClass     string
	targetNamespace        string
	targetKubeconfig       string
	targetIssuerNamespaced bool
	targetIssuerName       string
	metricsAddr            string
	enableLeaderElection   bool
	tlsRepsect             bool
}

var (
	setupLog = ctrl.Log.WithName("setup")
)

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
			if options.targetNamespace == "" {
				logger.Error(fmt.Errorf("target namespace must be defined"), "")
				os.Exit(1)
			}

			manager, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
				Metrics: metricsserver.Options{
					BindAddress: options.metricsAddr,
				},
				LeaderElection:         options.enableLeaderElection,
				LeaderElectionID:       "2c123jea.buttah.cloud",
				HealthProbeBindAddress: ":10080",
				NewClient: func(config *rest.Config, options client.Options) (client.Client, error) {
					options.Cache.Unstructured = true
					return client.New(config, options)
				},
			})
			if err != nil {
				logger.Error(err, "unable to start manager")
				os.Exit(1)
			}

			_ = manager.AddReadyzCheck("ping", healthz.Ping)
			_ = manager.AddHealthzCheck("ping", healthz.Ping)

			ctx := ctrl.SetupSignalHandler()

			if err = (&controller.PropagationController{
				Client:       manager.GetClient(),
				TargetClient: targetClient,
				Log:          ctrl.Log.WithName("controllers").WithName("Ingress"),
				Recorder:     manager.GetEventRecorderFor("ingress-controller"),
				Options: controller.PropagationControllerOptions{
					Identifier:             options.identifier,
					IngressClassName:       options.ingressClass,
					TargetIngressClassName: options.targetIngressClass,
					ControllerClassName:    options.controllerClass,
					TargetNamespace:        options.targetNamespace,
					TargetIssuerNamespaced: options.targetIssuerNamespaced,
					TargetIssuerName:       options.targetIssuerName,
					TLSrespect:             options.tlsRepsect,
				},
			}).SetupWithManager(ctx, manager); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "Ingress")
				os.Exit(1)
			}

			setupLog.Info("propagation manager start serving")

			if err = manager.Start(ctx); err != nil {
				setupLog.Error(err, "problem running manager")
				os.Exit(1)
			}

			return nil
		},
	}

	rootCommand.PersistentFlags().StringVar(&options.ingressClass, "ingress-class", options.ingressClass, "ingress class name")
	rootCommand.PersistentFlags().StringVar(&options.controllerClass, "controller-class", options.controllerClass, "controller class name")
	rootCommand.PersistentFlags().IntVarP(&options.logLevel, "log-level", "v", options.logLevel, "numeric log level")
	rootCommand.PersistentFlags().StringVar(&options.targetIngressClass, "target-ingress-class", options.targetIngressClass, "Ingress Class on target cluster")
	rootCommand.PersistentFlags().StringVar(&options.identifier, "identifier", options.identifier, "propagator identifier, if multiple propagators sync to the same target namespace, this should be different for each")
	rootCommand.PersistentFlags().StringVar(&options.targetNamespace, "target-namespace", options.targetNamespace, "namespace on target cluster, where manifests are synced to")
	rootCommand.PersistentFlags().StringVar(&options.targetKubeconfig, "target-kubeconfig", options.targetKubeconfig, "namespace on target cluster, where manifests are synced to")
	rootCommand.PersistentFlags().StringVar(&options.targetIssuerName, "target-issuer-name", options.targetIssuerName, "name of issuer added as cert-manager annotation on target cluster")
	rootCommand.PersistentFlags().BoolVar(&options.targetIssuerNamespaced, "target-issuer-namespaced", false, "name of issuer added as cert-manager annotation on target cluster")
	rootCommand.PersistentFlags().StringVar(&options.metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	rootCommand.PersistentFlags().BoolVar(&options.enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	rootCommand.PersistentFlags().BoolVar(&options.tlsRepsect, "tls-respect", false, "Respect TLS Spec on ingress objects, if an issuer is defined the TLS spec is added anyway")
	err := rootCommand.Execute()
	if err != nil {
		panic(err)
	}
}
