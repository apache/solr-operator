/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/apache/solr-operator/version"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"strings"

	zkv1beta1 "github.com/pravega/zookeeper-operator/pkg/apis"
	k8sRuntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

const (
	EnvOperatorPodName      = "POD_NAME"
	EnvOperatorPodNamespace = "POD_NAMESPACE"
)

var (
	scheme   = k8sRuntime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	namespace string
	name      string

	// Operator scope
	watchNamespaces string

	// External Operator dependencies
	useZookeeperCRD bool

	// mTLS information
	clientSkipVerify  bool
	clientCertPath    string
	clientCertKeyPath string
	caCertPath        string
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = solrv1beta1.AddToScheme(scheme)
	_ = zkv1beta1.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
	flag.BoolVar(&useZookeeperCRD, "zk-operator", true, "The operator will not use the zk operator & crd when this flag is set to false.")
	flag.StringVar(&watchNamespaces, "watch-namespaces", "", "The comma-separated list of namespaces to watch. If an empty string (default) is provided, the operator will watch the entire Kubernetes cluster.")

	flag.BoolVar(&clientSkipVerify, "tls-skip-verify-server", true, "Controls whether a client verifies the server's certificate chain and host name. If true (insecure), TLS accepts any certificate presented by the server and any host name in that certificate.")
	flag.StringVar(&clientCertPath, "tls-client-cert-path", "", "Path where a TLS client cert can be found")
	flag.StringVar(&clientCertKeyPath, "tls-client-cert-key-path", "", "Path where a TLS client cert key can be found")

	flag.StringVar(&caCertPath, "tls-ca-cert-path", "", "Path where a Certificate Authority (CA) cert in PEM format can be found")

	flag.Parse()
}

func main() {
	namespace = os.Getenv(EnvOperatorPodNamespace)
	if len(namespace) == 0 {
		//log.Fatalf("must set env (%s)", constants.EnvOperatorPodNamespace)
	}
	name = os.Getenv(EnvOperatorPodName)
	if len(name) == 0 {
		//log.Fatalf("must set env (%s)", constants.EnvOperatorPodName)
	}

	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.Logger(true))

	fullVersion := version.Version
	if version.VersionSuffix != "" {
		fullVersion += "-" + version.VersionSuffix
	}
	setupLog.Info(fmt.Sprintf("solr-operator Version: %v", fullVersion))
	setupLog.Info(fmt.Sprintf("solr-operator Git SHA: %s", version.GitSHA))
	setupLog.Info(fmt.Sprintf("solr-operator Build Time: %s", version.BuildTime))
	setupLog.Info(fmt.Sprintf("Go Version: %v", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s / %s", runtime.GOOS, runtime.GOARCH))

	// When the operator is started to watch resources in a specific set of namespaces, we use the MultiNamespacedCacheBuilder cache.
	// In this scenario, it is also suggested to restrict the provided authorization to this namespace by replacing the default
	// ClusterRole and ClusterRoleBinding to Role and RoleBinding respectively
	// For further information see the kubernetes documentation about
	// Using [RBAC Authorization](https://kubernetes.io/docs/reference/access-authn-authz/rbac/).
	var managerWatchCache cache.NewCacheFunc
	if watchNamespaces != "" {
		setupLog.Info(fmt.Sprintf("Managing for Namespaces: %s", watchNamespaces))
		ns := strings.Split(watchNamespaces, ",")
		for i := range ns {
			ns[i] = strings.TrimSpace(ns[i])
		}
		managerWatchCache = cache.MultiNamespacedCacheBuilder(ns)
	} else {
		setupLog.Info("Managing for the entire cluster.")
		managerWatchCache = (cache.NewCacheFunc)(nil)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
		NewCache:           managerWatchCache,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	controllers.UseZkCRD(useZookeeperCRD)

	if err = initMTLSConfig(); err != nil {
		os.Exit(1)
	}

	if err = (&controllers.SolrCloudReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("SolrCloud"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SolrCloud")
		os.Exit(1)
	}
	if err = (&controllers.SolrBackupReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("SolrBackup"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SolrBackup")
		os.Exit(1)
	}
	if err = (&controllers.SolrPrometheusExporterReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("SolrPrometheusExporter"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SolrPrometheusExporter")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func initMTLSConfig() error {
	if clientCertPath != "" {
		setupLog.Info("mTLS config", "clientSkipVerify", clientSkipVerify, "clientCertPath", clientCertPath,
			"clientCertKeyPath", clientCertKeyPath, "caCertPath", caCertPath)

		// Load client cert information from files
		clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientCertKeyPath)
		if err != nil {
			setupLog.Error(err, "Error loading clientCert pair for mTLS transport", "certPath", clientCertPath, "keyPath", clientCertKeyPath)
			return err
		}

		mTLSTransport := http.DefaultTransport.(*http.Transport).Clone()
		mTLSTransport.TLSClientConfig = &tls.Config{Certificates: []tls.Certificate{clientCert}, InsecureSkipVerify: clientSkipVerify}

		// Add the rootCA if one is provided
		if caCertPath != "" {
			if caCertBytes, err := ioutil.ReadFile(caCertPath); err == nil {
				caCertPool := x509.NewCertPool()
				caCertPool.AppendCertsFromPEM(caCertBytes)
				mTLSTransport.TLSClientConfig.ClientCAs = caCertPool
				setupLog.Info("Configured the custom CA pem for the mTLS transport", "path", caCertPath)
			} else {
				setupLog.Error(err, "Cannot read provided CA pem for mTLS transport", "path", caCertPath)
				return err
			}
		}

		solr_api.SetMTLSHttpClient(&http.Client{Transport: mTLSTransport})
	}

	return nil
}
