package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/oliverbaehler/cloudflare-tunnel-ingress-controller/pkg/controller"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type rootCmdFlags struct {
	logger logr.Logger
	// for annotation on Ingress
	ingressClass string
	// Ingress class on loadbalancer cluster
	targetIngressClass string
	// for IngressClass.spec.controller
	namespace string
	// for identifying objects on parent cluster
	identifier string
	// Kubeconfig for parent cluster
	kubeconfig string

	logLevel int
}

func main() {
	var rootLogger = stdr.NewWithOptions(log.New(os.Stderr, "", log.LstdFlags), stdr.Options{LogCaller: stdr.All})

	options := rootCmdFlags{
		logger:          rootLogger.WithName("main"),
		ingressClass:    "cloudflare-tunnel",
		controllerClass: "strrl.dev/cloudflare-tunnel-ingress-controller",
		logLevel:        0,
		namespace:       "default",
	}

	crlog.SetLogger(rootLogger.WithName("controller-runtime"))

	rootCommand := cobra.Command{
		Use: "tunnel-controller",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			stdr.SetVerbosity(options.logLevel)
			logger := options.logger
			logger.Info("logging verbosity", "level", options.logLevel)

			cfg, err := config.GetConfig()
			if err != nil {
				logger.Error(err, "unable to get kubeconfig")
				os.Exit(1)
			}

			mgr, err := manager.New(cfg, manager.Options{})
			if err != nil {
				logger.Error(err, "unable to set up manager")
				os.Exit(1)
			}

			logger.Info("cloudflare-tunnel-ingress-controller start serving")
			err = controller.RegisterIngressController(logger, mgr,
				controller.IngressControllerOptions{
					Identifier:             options.identifier,
					IngressClassName:       options.ingressClass,
					TargetIngressClassName: options.targetIngressClass,
					ControllerClassName:    options.controllerClass,
				})
			if err != nil {
				return err
			}

			ticker := time.NewTicker(10 * time.Second)
			done := make(chan struct{})
			defer close(done)

			go func() {
				for {
					select {
					case <-done:
						return
					case _ = <-ticker.C:
						err := controller.CreateControlledCloudflaredIfNotExist(ctx, mgr.GetClient(), tunnelClient, options.namespace)
						if err != nil {
							logger.WithName("controlled-cloudflared").Error(err, "create controlled cloudflared")
						}
					}
				}
			}()

			// controller-runtime manager would graceful shutdown with signal by itself, no need to provide context
			return mgr.Start(context.Background())
		},
	}

	rootCommand.PersistentFlags().StringVar(&options.ingressClass, "ingress-class", options.ingressClass, "ingress class name")
	rootCommand.PersistentFlags().StringVar(&options.targetIngressClass, "ingress-class", options.targetIngressClass, "ingress class name")
	rootCommand.PersistentFlags().StringVar(&options.controllerClass, "controller-class", options.controllerClass, "controller class name")
	rootCommand.PersistentFlags().IntVarP(&options.logLevel, "log-level", "v", options.logLevel, "numeric log level")
	rootCommand.PersistentFlags().StringVar(&options.cloudflareAPIToken, "cloudflare-api-token", options.cloudflareAPIToken, "cloudflare api token")
	rootCommand.PersistentFlags().StringVar(&options.cloudflareAccountId, "cloudflare-account-id", options.cloudflareAccountId, "cloudflare account id")
	rootCommand.PersistentFlags().StringVar(&options.cloudflareTunnelName, "cloudflare-tunnel-name", options.cloudflareTunnelName, "cloudflare tunnel name")
	rootCommand.PersistentFlags().StringVar(&options.namespace, "namespace", options.namespace, "namespace to execute cloudflared connector")

	err := rootCommand.Execute()
	if err != nil {
		panic(err)
	}
}
