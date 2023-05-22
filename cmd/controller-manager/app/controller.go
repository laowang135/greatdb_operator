/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package app

import (
	"fmt"

	"time"

	clientset "greatdb-operator/pkg/client/clientset/versioned"
	greatdbInformer "greatdb-operator/pkg/client/informers/externalversions"
	"greatdb-operator/pkg/config"
	"greatdb-operator/pkg/controllers"
	deps "greatdb-operator/pkg/controllers/dependences"
	greatdb_api "greatdb-operator/pkg/greatdb-api"
	"greatdb-operator/pkg/utils/log"
	"greatdb-operator/pkg/utils/signals"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubeinformer "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/client-go/tools/clientcmd"
)

var (
	defaultResync  time.Duration
	tlsCertFile    string
	tlsKeyFile     string
	enableWebhooks bool
	Port           int
)

func init() {
	defaultResync = time.Second * 180
}

func AddFlag() *pflag.FlagSet {
	fss := pflag.NewFlagSet("config", pflag.ExitOnError)
	fss.StringVar(&config.KubeConfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	fss.StringVar(&config.MasterHost, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	fss.IntVar(&config.LogLever, "V", 2, "Log level")
	fss.StringVar(&config.ManagerBy, "managerBy", "", "Log level")

	fss.StringVar(&tlsKeyFile, "tlsKeyFile", "/etc/certs/key.pem", "Api service key")
	fss.StringVar(&tlsCertFile, "tlsCertFile", "/etc/certs/cert.pem", "api service certificate")
	fss.BoolVar(&enableWebhooks, "webhooks", false, "Open the webhooks service")
	fss.IntVar(&Port, "port", 8008, "webhooks service listening port")
	return fss

}

func NewControllerCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use: "greatdb-operator",
		Long: `Greatdb operator is a project developed based on kubernetes 
		to manage the deployment, maintenance and other functions 
		of greatdb databases on the kubernetes platform`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any argument,got %q", cmd.CommandPath(), args)
				}
			}
			return nil
		},
		Example: `
        1、Print version number
           greatdb-operator -v or greatdb-operator --version
        2、set log level
           greatdb-operator --V=3`,

		Version: "v0.1",
	}
	fs := cmd.Flags()
	fs.AddFlagSet(AddFlag())
	log.InitializeLogging("greatdb-operator")
	log.Log.SetVerbosityLevel(config.LogLever)

	return cmd

}

func LabelKubeWithOptions() []kubeinformer.SharedInformerOption {
	var kubeoptions []kubeinformer.SharedInformerOption

	tweakListOptionsFunc := func(options *metav1.ListOptions) {

		if config.ManagerBy != "" {
			if options.LabelSelector != "" {
				options.LabelSelector += ",app.kubernetes/managed-by=" + config.ManagerBy
			} else {

				options.LabelSelector += "app.kubernetes/managed-by=" + config.ManagerBy
			}

		}

		if options.LabelSelector != "" {
			options.LabelSelector += ",app.kubernetes.io/name=" + config.ServiceType
		} else {

			options.LabelSelector += "app.kubernetes.io/name=" + config.ServiceType
		}

	}

	kubeoptions = append(kubeoptions, kubeinformer.WithTweakListOptions(tweakListOptionsFunc))

	return kubeoptions

}

func greatdbWithOptions() []greatdbInformer.SharedInformerOption {

	var greatdbOptions []greatdbInformer.SharedInformerOption
	if config.ManagerBy != "" {
		tweakListOptionsFunc := func(options *metav1.ListOptions) {
			if options.LabelSelector != "" {
				options.LabelSelector += ",app.kubernetes/managed-by=" + config.ManagerBy
			} else {

				options.LabelSelector += "app.kubernetes/managed-by=" + config.ManagerBy
			}
		}
		greatdbOptions = append(greatdbOptions, greatdbInformer.WithTweakListOptions(tweakListOptionsFunc))
	}

	return greatdbOptions

}

func ApiVersion(client *kubernetes.Clientset) error {

	// APIResources
	groups, err := client.ServerGroups()
	if err != nil {
		log.Log.Error(err.Error())
		return err
	}
	pdbHas := false
	for _, group := range groups.Groups {
		if group.Name != "policy" {
			continue
		}
		resource, err := client.ServerResourcesForGroupVersion(group.PreferredVersion.GroupVersion)
		if err != nil {
			log.Log.Error(err.Error())
			return err
		}

		for _, api := range resource.APIResources {
			if api.Name == "poddisruptionbudgets" {
				config.ApiVersion.PDB = group.PreferredVersion.GroupVersion
				pdbHas = true
				break
			}
		}

		if pdbHas {
			break
		}

	}
	// ServerVersion
	ver, err := client.ServerVersion()
	if err != nil {
		log.Log.Error(err.Error())
		return err
	}

	config.ServerVersion = ver.String()
	log.Log.Infof("k8s version: %s", config.ServerVersion)
	return nil

}

func run() error {

	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(config.MasterHost, config.KubeConfig)
	if err != nil {
		log.Log.Error(err.Error())
		return err
	}

	kubeclient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Log.Error(err.Error())
		return err
	}

	err = ApiVersion(kubeclient)
	if err != nil {
		log.Log.Error(err.Error())
		return err
	}
	greatdbClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		log.Log.Error(err.Error())
		return err
	}

	kubeoptions := LabelKubeWithOptions()
	kubeLabelInformerFactory := kubeinformer.NewSharedInformerFactoryWithOptions(kubeclient, defaultResync, kubeoptions...)
	kubeInformerFactory := kubeinformer.NewSharedInformerFactory(kubeclient, defaultResync)

	greatdbOptions := greatdbWithOptions()
	greatdbInformerFactory := greatdbInformer.NewSharedInformerFactoryWithOptions(greatdbClient, defaultResync, greatdbOptions...)

	clientSet := deps.NewClientSet(kubeclient, greatdbClient)
	// Register listening resources and obtain Lister objects
	Listers := deps.NewListers(kubeLabelInformerFactory, kubeInformerFactory, greatdbInformerFactory)

	// start apiserver

	go greatdb_api.Server(greatdbClient, kubeclient, tlsKeyFile, tlsCertFile, Port, enableWebhooks, stopCh)

	ctrl := controllers.NewControllers(clientSet, Listers, kubeLabelInformerFactory, kubeInformerFactory, greatdbInformerFactory, stopCh)

	kubeLabelInformerFactory.Start(stopCh)
	kubeInformerFactory.Start(stopCh)
	greatdbInformerFactory.Start(stopCh)

	// start controller
	ctrl.Run()

	return nil
}
