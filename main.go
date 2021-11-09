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
	"github.com/apache/solr-operator/controllers/util/solr_api"
	"github.com/apache/solr-operator/controllers/zk_api"
	"github.com/apache/solr-operator/version"
	"github.com/fsnotify/fsnotify"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sort"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	k8sRuntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	solrv1beta1 "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers"
	//+kubebuilder:scaffold:imports
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
	clientCertWatch   bool

	// Ref to the active client certificate, will get updated when the secret changes if watch enabled
	clientCertificate *tls.Certificate
)

const (
	EnvOperatorPodName      = "POD_NAME"
	EnvOperatorPodNamespace = "POD_NAMESPACE"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(solrv1beta1.AddToScheme(scheme))

	utilruntime.Must(zk_api.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme

	flag.BoolVar(&useZookeeperCRD, "zk-operator", true, "The operator will not use the zk operator & crd when this flag is set to false.")
	flag.StringVar(&watchNamespaces, "watch-namespaces", "", "The comma-separated list of namespaces to watch. If an empty string (default) is provided, the operator will watch the entire Kubernetes cluster.")

	flag.BoolVar(&clientSkipVerify, "tls-skip-verify-server", true, "Controls whether a client verifies the server's certificate chain and host name. If true (insecure), TLS accepts any certificate presented by the server and any host name in that certificate.")
	flag.StringVar(&clientCertPath, "tls-client-cert-path", "", "Path where a TLS client cert can be found")
	flag.StringVar(&clientCertKeyPath, "tls-client-cert-key-path", "", "Path where a TLS client cert key can be found")

	flag.StringVar(&caCertPath, "tls-ca-cert-path", "", "Path where a Certificate Authority (CA) cert in PEM format can be found")
	flag.BoolVar(&clientCertWatch, "tls-watch-cert", true, "Controls whether the operator performs a hot reload of the mTLS when it gets updated; set to false to disable watching for updates to the TLS cert.")

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
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	fullVersion := version.Version
	if version.VersionSuffix != "" {
		fullVersion += "-" + version.VersionSuffix
	}
	setupLog.Info(fmt.Sprintf("solr-operator Version: %v", fullVersion))
	setupLog.Info(fmt.Sprintf("solr-operator Git SHA: %s", version.GitSHA))
	setupLog.Info(fmt.Sprintf("solr-operator Build Time: %s", version.BuildTime))
	setupLog.Info(fmt.Sprintf("Go Version: %v", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s / %s", runtime.GOOS, runtime.GOARCH))

	operatorOptions := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "88488bdc.solr.apache.org",
	}

	/*
		When the operator is started to watch resources in a specific set of namespaces, we use the MultiNamespacedCacheBuilder cache.
		In this scenario, it is also suggested to restrict the provided authorization to this namespace by replacing the default
		ClusterRole and ClusterRoleBinding to Role and RoleBinding respectively
		For further information see the kubernetes documentation about
		Using [RBAC Authorization](https://kubernetes.io/docs/reference/access-authn-authz/rbac/).

		When watching multiple namespaces with leader election enabled, the leader election will use the lowest-sorted namespace.
		It is likely safer to run individual solr operators per-namespace, to ensure leader election is working as expected no matter what.
	*/
	if watchNamespaces != "" {
		setupLog.Info(fmt.Sprintf("Managing for Namespaces: %s", watchNamespaces))
		ns := strings.Split(watchNamespaces, ",")
		for i := range ns {
			ns[i] = strings.TrimSpace(ns[i])
		}
		sort.Strings(ns)
		operatorOptions.NewCache = cache.MultiNamespacedCacheBuilder(ns)

		operatorOptions.LeaderElectionNamespace = ns[0]
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), operatorOptions)
	if err != nil {
		setupLog.Error(err, "unable to start solr operator")
		os.Exit(1)
	}

	controllers.UseZkCRD(useZookeeperCRD)

	// watch TLS files for update
	if clientCertPath != "" {
		var watcher *fsnotify.Watcher
		if clientCertWatch {
			watcher, err = fsnotify.NewWatcher()
			if err != nil {
				setupLog.Error(err, "Create new file watcher failed")
				os.Exit(1)
			}
			defer watcher.Close()
		}

		if err = initMTLSConfig(watcher); err != nil {
			os.Exit(1)
		}
	}

	if err = (&controllers.SolrCloudReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SolrCloud")
		os.Exit(1)
	}
	if err = (&controllers.SolrPrometheusExporterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SolrPrometheusExporter")
		os.Exit(1)
	}
	if err = (&controllers.SolrBackupReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Config: mgr.GetConfig(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SolrBackup")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// Setup for mTLS with Solr pods with hot reload support using the fsnotify Watcher
func initMTLSConfig(watcher *fsnotify.Watcher) error {
	setupLog.Info("mTLS config", "clientSkipVerify", clientSkipVerify, "clientCertPath", clientCertPath,
		"clientCertKeyPath", clientCertKeyPath, "caCertPath", caCertPath, "clientCertWatch", clientCertWatch)

	// Load client cert information from files
	clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientCertKeyPath)
	if err != nil {
		setupLog.Error(err, "Error loading clientCert pair for mTLS transport", "certPath", clientCertPath, "keyPath", clientCertKeyPath)
		return err
	}

	if watcher != nil {
		// If the cert file is a symlink (which is the case when loaded from a secret), then we need to re-add the watch after the
		// Right now, it will always be a symlink, so this is just future proofing when the cert gets loaded from a CSI driver
		isSymlink := false
		clientCertFile, _ := filepath.EvalSymlinks(clientCertPath)
		if clientCertFile != clientCertPath {
			isSymlink = true
		}

		// Watch cert files for updates
		go func() {
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}
					// If the cert was loaded from a secret, then the path will be for a symlink and an update comes in as a REMOVE event
					// otherwise, look for a write to the real file
					if ((isSymlink && event.Op&fsnotify.Remove == fsnotify.Remove) || (event.Op&fsnotify.Write == fsnotify.Write)) && event.Name == clientCertPath {
						clientCertFile, _ := filepath.EvalSymlinks(clientCertPath)
						setupLog.Info("mTLS cert file updated", "certPath", clientCertPath, "certFile", clientCertFile)

						clientCert, err = tls.LoadX509KeyPair(clientCertPath, clientCertKeyPath)
						if err == nil {
							// update our global client certificate to the new one
							clientCertificate = &clientCert
							setupLog.Info("Updated mTLS Http Client after update to cert", "certPath", clientCertPath)
						} else {
							// will keep using the old cert, which eventually will cause failures when the old cert expires
							setupLog.Error(err, "Error loading clientCert pair (after update) for mTLS transport", "certPath", clientCertPath, "keyPath", clientCertKeyPath)
						}

						// If the symlink we were watching was removed, re-add the watch
						if event.Op&fsnotify.Remove == fsnotify.Remove {
							err = watcher.Add(clientCertPath)
							if err != nil {
								setupLog.Error(err, "Re-add fsnotify watch for cert failed", "certPath", clientCertPath)
							} else {
								setupLog.Info("Re-added watch for symlink to mTLS cert file", "certPath", clientCertPath, "certFile", clientCertFile)
							}
						}
					}
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					setupLog.Error(err, "fsnotify error")
				}
			}
		}()

		err = watcher.Add(clientCertPath)
		if err != nil {
			setupLog.Error(err, "Add fsnotify watch for cert failed", "certPath", clientCertPath)
			return err
		}
		setupLog.Info("Added fsnotify watch for mTLS cert file", "certPath", clientCertPath, "certFile", clientCertFile, "isSymlink", isSymlink)
	} else {
		setupLog.Info("Watch for mTLS cert updates disabled", "certPath", clientCertPath)
	}

	clientCertificate = &clientCert
	mTLSTransport, err := buildTLSTransport()
	if err != nil {
		return err
	}
	solr_api.SetMTLSHttpClient(&http.Client{Transport: mTLSTransport})

	return nil
}

func buildTLSTransport() (*http.Transport, error) {
	mTLSTransport := http.DefaultTransport.(*http.Transport).Clone()
	mTLSTransport.TLSClientConfig = &tls.Config{GetClientCertificate: getClientCertificate, InsecureSkipVerify: clientSkipVerify}

	// Add the rootCA if one is provided
	if caCertPath != "" {
		if caCertBytes, err := ioutil.ReadFile(caCertPath); err == nil {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(caCertBytes)
			mTLSTransport.TLSClientConfig.ClientCAs = caCertPool
			setupLog.Info("Configured the custom CA pem for the mTLS transport", "path", caCertPath)
		} else {
			setupLog.Error(err, "Cannot read provided CA pem for mTLS transport", "path", caCertPath)
			return nil, err
		}
	}

	return mTLSTransport, nil
}

func getClientCertificate(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
	return clientCertificate, nil
}
