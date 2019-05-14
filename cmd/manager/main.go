/*
Copyright 2019 Bloomberg Finance LP.

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

package main

import (
	"flag"
	"fmt"
	"github.com/bloomberg/solr-operator/pkg/apis"
	"github.com/bloomberg/solr-operator/pkg/controller"
	"github.com/bloomberg/solr-operator/pkg/controller/solrcloud"
	"github.com/bloomberg/solr-operator/pkg/version"
	"github.com/bloomberg/solr-operator/pkg/webhook"
	"github.com/coreos/etcd-operator/pkg/util/constants"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

var (
	namespace string
	name      string

	useEtcdOperator      bool
	useZookeeperOperator bool

	ingressBaseDomain string
)

func init() {
	flag.BoolVar(&useEtcdOperator, "etcd-operator", true, "The operator will not use the etcd-operator when this flag is set to false.")
	flag.BoolVar(&useZookeeperOperator, "zk-operator", true, "The operator will not use the zk-operator when this flag is set to false.")
	flag.StringVar(&ingressBaseDomain, "ingress-base-domain", "", "The operator will use this base domain for host matching in an ingress for the cloud.")
	flag.Parse()
}

func main() {
	namespace = os.Getenv(constants.EnvOperatorPodNamespace)
	if len(namespace) == 0 {
		//log.Fatalf("must set env (%s)", constants.EnvOperatorPodNamespace)
	}
	name = os.Getenv(constants.EnvOperatorPodName)
	if len(name) == 0 {
		//log.Fatalf("must set env (%s)", constants.EnvOperatorPodName)
	}

	var metricsAddr string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.Parse()
	logf.SetLogger(logf.ZapLogger(false))
	log := logf.Log.WithName("entrypoint")

	log.Info(fmt.Sprintf("solr-operator Version: %v", version.Version))
	log.Info(fmt.Sprintf("solr-operator Git SHA: %s", version.GitSHA))
	log.Info(fmt.Sprintf("Go Version: %v", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s / %s", runtime.GOOS, runtime.GOARCH))

	// Get a config to talk to the apiserver
	log.Info("setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	log.Info("setting up manager")
	mgr, err := manager.New(cfg, manager.Options{MetricsBindAddress: metricsAddr})
	if err != nil {
		log.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	log.Info("setting up scheme")
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable add Solr Operator APIs to scheme")
		os.Exit(1)
	}

	solrcloud.SetIngressBaseUrl(ingressBaseDomain)
	// Setup all Controllers
	log.Info("Setting up controller")
	if err := controller.AddToManager(mgr, useZookeeperOperator, useEtcdOperator); err != nil {
		log.Error(err, "unable to register controllers to the manager")
		os.Exit(1)
	}

	log.Info("setting up webhooks")
	if err := webhook.AddToManager(mgr); err != nil {
		log.Error(err, "unable to register webhooks to the manager")
		os.Exit(1)
	}

	// Start the Cmd
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
