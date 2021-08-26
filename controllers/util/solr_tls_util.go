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

package util

import (
	"fmt"
	solr "github.com/apache/solr-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"strconv"
	"strings"
)

const (
	SolrTlsCertMd5Annotation       = "solr.apache.org/tlsCertMd5"
	SolrClientTlsCertMd5Annotation = "solr.apache.org/tlsClientCertMd5"
	DefaultKeyStorePath            = "/var/solr/tls"
	DefaultClientKeyStorePath      = "/var/solr/client-tls"
	DefaultWritableKeyStorePath    = "/var/solr/tls/pkcs12"
	TLSCertKey                     = "tls.crt"
	DefaultTrustStorePath          = "/var/solr/tls-truststore"
	DefaultClientTrustStorePath    = "/var/solr/client-tls-truststore"
	InitdbPath                     = "/docker-entrypoint-initdb.d"
	DefaultPkcs12KeystoreFile      = "keystore.p12"
	DefaultPkcs12TruststoreFile    = "truststore.p12"
	DefaultKeystorePasswordFile    = "keystore-password"
)

// Helper struct for holding server and/or client cert config based on options from the CRD as well as variables resolved during reconciliation
// This struct is intended for internal use only and is only exposed outside the package so that the controllers can access
type TLSCerts struct {
	// Server cert config
	ServerConfig *TLSConfig
	// Client cert config
	ClientConfig *TLSConfig
	// Image used for initContainers that help configure the TLS settings
	InitContainerImage *solr.ContainerImage
}

// Holds TLS options from the user config as well as other config properties determined during reconciliation
// This struct is intended for internal use only and is only exposed outside the package so that the controllers can access
type TLSConfig struct {
	// TLS options provided by the user in the CRD definition
	Options *solr.SolrTLSOptions
	// Flag to indicate if we need to convert the provided keystore into the p12 format needed by Java
	NeedsPkcs12InitContainer bool
	// The MD5 hash of the cert, used for restarting Solr pods after the cert updates if so desired
	CertMd5        string
	KeystorePath   string
	TruststorePath string
}

// Enrich the config for a SolrCloud StatefulSet to enable TLS, either loaded from a secret or
// a directory on the main pod containing per-pod specific TLS files. In the latter case, the "mounted dir"
// typically comes from an external agent (such as a cert-manager extension) or CSI driver that injects the
// pod-specific TLS files using mutating web hooks
func (tls *TLSCerts) enableTLSOnSolrCloudStatefulSet(stateful *appsv1.StatefulSet) {
	// Add the SOLR_SSL_* vars to the main container's environment
	mainContainer := &stateful.Spec.Template.Spec.Containers[0]
	mainContainer.Env = append(mainContainer.Env, tls.serverEnvVars()...)

	serverCert := tls.ServerConfig
	if serverCert.Options.PKCS12Secret != nil {
		// Cert comes from a secret, so setup the pod template to mount the secret
		var tlsAnnotations map[string]string
		if serverCert.Options.RestartOnTLSSecretUpdate && serverCert.CertMd5 != "" {
			tlsAnnotations = make(map[string]string, 1)
			tlsAnnotations[SolrTlsCertMd5Annotation] = tls.ServerConfig.CertMd5
		}
		serverCert.mountTLSSecretOnPodTemplate(&stateful.Spec.Template, tlsAnnotations)

		// mount the client certificate from a different secret
		clientCert := tls.ClientConfig
		if clientCert != nil && clientCert.Options.PKCS12Secret != nil {
			var tlsClientAnnotations map[string]string
			if clientCert.Options.RestartOnTLSSecretUpdate && clientCert.CertMd5 != "" {
				tlsClientAnnotations = make(map[string]string, 1)
				tlsClientAnnotations[SolrClientTlsCertMd5Annotation] = clientCert.CertMd5
			}
			clientCert.mountTLSSecretOnPodTemplate(&stateful.Spec.Template, tlsClientAnnotations)
		}

	} else if serverCert.Options.MountedTLSDir != nil {
		// the TLS files come from some auto-mounted directory on the main container
		mountInitDbIfNeeded(stateful)
		// use an initContainer to create the wrapper script in the initdb
		stateful.Spec.Template.Spec.InitContainers = append(stateful.Spec.Template.Spec.InitContainers, tls.generateTLSInitdbScriptInitContainer())
	}
}

// Enrich the config for a Prometheus Exporter Deployment to allow the exporter to make requests to TLS enabled Solr pods
func (tls *TLSCerts) enableTLSOnExporterDeployment(deployment *appsv1.Deployment) {
	clientCert := tls.ClientConfig

	// Add the SOLR_SSL_* vars to the main container's environment
	mainContainer := &deployment.Spec.Template.Spec.Containers[0]
	mainContainer.Env = append(mainContainer.Env, clientCert.clientEnvVars()...)
	// the exporter process doesn't read the SOLR_SSL_* env vars, so we need to pass them via JAVA_OPTS
	appendJavaOptsToEnv(mainContainer, clientCert.clientJavaOpts())

	if clientCert.Options.PKCS12Secret != nil || clientCert.Options.TrustStoreSecret != nil {
		// Cert comes from a secret, so setup the pod template to mount the secret
		var tlsAnnotations map[string]string
		if clientCert.Options.RestartOnTLSSecretUpdate && clientCert.CertMd5 != "" {
			tlsAnnotations = make(map[string]string, 1)
			tlsAnnotations[SolrClientTlsCertMd5Annotation] = clientCert.CertMd5
		}
		clientCert.mountTLSSecretOnPodTemplate(&deployment.Spec.Template, tlsAnnotations)
	} else if clientCert.Options.MountedTLSDir != nil {
		// volumes and mounts for TLS when using the mounted dir option
		clientCert.mountTLSWrapperScriptAndInitContainer(deployment, tls.InitContainerImage)
	}
}

// Configures a pod template (either StatefulSet or Deployment) to mount the TLS files from a secret
func (tls *TLSConfig) mountTLSSecretOnPodTemplate(template *corev1.PodTemplateSpec, tlsAnnotations map[string]string) *corev1.Container {
	// Add the SOLR_SSL_* vars to the main container's environment
	mainContainer := &template.Spec.Containers[0]

	// the TLS files are mounted from a secret, setup the volumes and mounts
	vols, mounts := tls.volumesAndMounts()

	// We need an initContainer to convert a TLS cert into the pkcs12 format Java wants (using openssl)
	// but openssl cannot write to the /var/solr/tls directory because of the way secret mounts work
	// so we need to mount an empty directory to write pkcs12 keystore into
	if tls.NeedsPkcs12InitContainer {
		vols = append(vols, corev1.Volume{Name: "pkcs12", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}})
		mounts = append(mounts, corev1.VolumeMount{Name: "pkcs12", ReadOnly: false, MountPath: DefaultWritableKeyStorePath})
		pkcs12InitContainer := tls.generatePkcs12InitContainer(tls.Options, mainContainer.Image, mainContainer.ImagePullPolicy, mounts)
		template.Spec.InitContainers = append(template.Spec.InitContainers, pkcs12InitContainer)
	}
	template.Spec.Volumes = append(template.Spec.Volumes, vols...)
	mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, mounts...)

	// track the MD5 of the TLS cert (from secret) to trigger restarts if the cert changes
	if tlsAnnotations != nil {
		if template.Annotations == nil {
			template.Annotations = make(map[string]string, 1)
		}
		for key, value := range tlsAnnotations {
			template.Annotations[key] = value
		}
	}

	return mainContainer
}

// Get a list of volumes for the keystore and optionally a truststore loaded from a TLS secret
func (tls *TLSConfig) volumesAndMounts() ([]corev1.Volume, []corev1.VolumeMount) {
	optional := false
	defaultMode := int32(0664)
	vols := []corev1.Volume{}
	mounts := []corev1.VolumeMount{}
	keystoreSecretName := ""

	opts := tls.Options
	if opts.PKCS12Secret != nil {
		keystoreSecretName = opts.PKCS12Secret.Name
		vols = append(vols, corev1.Volume{
			Name: "keystore",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  opts.PKCS12Secret.Name,
					DefaultMode: &defaultMode,
					Optional:    &optional,
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: "keystore", ReadOnly: true, MountPath: tls.KeystorePath})
	}

	// if they're using a different truststore other than the keystore, but don't mount an additional volume
	// if it's just pointing at the same secret
	if opts.TrustStoreSecret != nil && opts.TrustStoreSecret.Name != keystoreSecretName {
		vols = append(vols, corev1.Volume{
			Name: "truststore",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  opts.TrustStoreSecret.Name,
					DefaultMode: &defaultMode,
					Optional:    &optional,
				},
			},
		})
		mounts = append(mounts, corev1.VolumeMount{Name: "truststore", ReadOnly: true, MountPath: tls.TruststorePath})
	}

	return vols, mounts
}

// Get the SOLR_SSL_* env vars for enabling TLS on Solr pods
func (tls *TLSCerts) serverEnvVars() []corev1.EnvVar {
	opts := tls.ServerConfig.Options

	// Determine the correct values for the SOLR_SSL_WANT_CLIENT_AUTH and SOLR_SSL_NEED_CLIENT_AUTH vars
	wantClientAuth := "false"
	needClientAuth := "false"
	if opts.ClientAuth == solr.Need {
		needClientAuth = "true"
	} else if opts.ClientAuth == solr.Want {
		wantClientAuth = "true"
	}

	envVars := []corev1.EnvVar{
		{
			Name:  "SOLR_SSL_ENABLED",
			Value: "true",
		},
		{
			Name:  "SOLR_SSL_WANT_CLIENT_AUTH",
			Value: wantClientAuth,
		},
		{
			Name:  "SOLR_SSL_NEED_CLIENT_AUTH",
			Value: needClientAuth,
		},
		{
			Name:  "SOLR_SSL_CLIENT_HOSTNAME_VERIFICATION",
			Value: strconv.FormatBool(opts.VerifyClientHostname),
		},
		{
			Name:  "SOLR_SSL_CHECK_PEER_NAME",
			Value: strconv.FormatBool(opts.CheckPeerName),
		},
	}

	// keystore / truststore come from either a mountedTLSDir or sourced from a secret mounted on the pod
	if opts.MountedTLSDir != nil {
		// TLS files are mounted by some external agent
		envVars = append(envVars, corev1.EnvVar{Name: "SOLR_SSL_KEY_STORE", Value: mountedTLSKeystorePath(opts.MountedTLSDir)})
		envVars = append(envVars, corev1.EnvVar{Name: "SOLR_SSL_TRUST_STORE", Value: mountedTLSTruststorePath(opts.MountedTLSDir)})

		// Was a client cert mounted too?
		if tls.ClientConfig != nil && tls.ClientConfig.Options.MountedTLSDir != nil {
			clientDir := tls.ClientConfig.Options.MountedTLSDir
			// passwords get exported from files in the TLS dir using an initdb wrapper script
			if clientDir.KeystoreFile != "" {
				envVars = append(envVars, corev1.EnvVar{Name: "SOLR_SSL_CLIENT_KEY_STORE", Value: mountedTLSKeystorePath(clientDir)})
			}
			if clientDir.TruststoreFile != "" {
				envVars = append(envVars, corev1.EnvVar{Name: "SOLR_SSL_CLIENT_TRUST_STORE", Value: mountedTLSTruststorePath(clientDir)})
			}
		}
	} else {
		// keystore / truststore + passwords come from a secret
		envVars = append(envVars, tls.ServerConfig.keystoreEnvVars("SOLR_SSL_KEY_STORE")...)
		envVars = append(envVars, tls.ServerConfig.truststoreEnvVars("SOLR_SSL_TRUST_STORE")...)
		// TODO: wire-up a client cert if needed
	}

	return envVars
}

// Set the SOLR_SSL_* for a Solr client process, e.g. the Exporter, which only needs a subset of SSL vars that a Solr pod would need
func (tls *TLSConfig) clientEnvVars() []corev1.EnvVar {
	opts := tls.Options

	envVars := []corev1.EnvVar{
		{
			Name:  "SOLR_SSL_CHECK_PEER_NAME",
			Value: strconv.FormatBool(opts.CheckPeerName),
		},
	}

	if opts.MountedTLSDir != nil {
		// passwords get exported from files in the TLS dir using an initdb wrapper script
		if opts.MountedTLSDir.KeystoreFile != "" {
			envVars = append(envVars, corev1.EnvVar{Name: "SOLR_SSL_CLIENT_KEY_STORE", Value: mountedTLSKeystorePath(opts.MountedTLSDir)})
		}
		if opts.MountedTLSDir.TruststoreFile != "" {
			envVars = append(envVars, corev1.EnvVar{Name: "SOLR_SSL_CLIENT_TRUST_STORE", Value: mountedTLSTruststorePath(opts.MountedTLSDir)})
		}
	}

	if opts.PKCS12Secret != nil {
		envVars = append(envVars, tls.keystoreEnvVars("SOLR_SSL_CLIENT_KEY_STORE")...)
		// if no additional truststore secret provided, just use the keystore for both
		if opts.TrustStoreSecret == nil {
			envVars = append(envVars, tls.keystoreEnvVars("SOLR_SSL_CLIENT_TRUST_STORE")...)
		}
	}

	if opts.TrustStoreSecret != nil {
		envVars = append(envVars, tls.truststoreEnvVars("SOLR_SSL_CLIENT_TRUST_STORE")...)
	}

	return envVars
}

func (tls *TLSConfig) keystoreEnvVars(varName string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  varName,
			Value: tls.keystoreFile(),
		},
		{
			Name:      varName + "_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: tls.Options.KeyStorePasswordSecret},
		},
	}
}

// the keystore path depends on whether we're just loading it from the secret or whether
// our initContainer has to generate it from the TLS secret using openssl
// this complexity is due to the secret mount directory not being writable
func (tls *TLSConfig) keystoreFile() string {
	var keystorePath string
	if tls.NeedsPkcs12InitContainer {
		keystorePath = DefaultWritableKeyStorePath
	} else {
		keystorePath = DefaultKeyStorePath
	}
	return keystorePath + "/" + DefaultPkcs12KeystoreFile
}

func (tls *TLSConfig) truststoreEnvVars(varName string) []corev1.EnvVar {
	opts := tls.Options
	keystoreSecretName := ""
	if opts.PKCS12Secret != nil {
		keystoreSecretName = opts.PKCS12Secret.Name
	}

	var truststoreFile string
	if opts.TrustStoreSecret != nil {
		if opts.TrustStoreSecret.Name != keystoreSecretName {
			// trust store is in a different secret, so will be mounted in a different dir
			truststoreFile = DefaultTrustStorePath + "/" + opts.TrustStoreSecret.Key
		} else {
			// trust store is a different key in the same secret as the keystore
			truststoreFile = DefaultKeyStorePath + "/" + DefaultPkcs12TruststoreFile
		}
	} else {
		// truststore is the same as the keystore
		truststoreFile = tls.keystoreFile()
	}

	var truststorePassFrom *corev1.EnvVarSource
	if opts.TrustStorePasswordSecret != nil {
		truststorePassFrom = &corev1.EnvVarSource{SecretKeyRef: opts.TrustStorePasswordSecret}
	} else {
		truststorePassFrom = &corev1.EnvVarSource{SecretKeyRef: opts.KeyStorePasswordSecret}
	}

	return []corev1.EnvVar{
		{
			Name:  varName,
			Value: truststoreFile,
		},
		{
			Name:      varName + "_PASSWORD",
			ValueFrom: truststorePassFrom,
		},
	}
}

// For the mounted dir approach, we need to customize the Prometheus exporter's entry point with a wrapper script
// that reads the keystore / truststore passwords from a file and passes them via JAVA_OPTS
func (tls *TLSConfig) mountTLSWrapperScriptAndInitContainer(deployment *appsv1.Deployment, initContainerImage *solr.ContainerImage) {
	opts := tls.Options
	mainContainer := &deployment.Spec.Template.Spec.Containers[0]

	volName := "tls-wrapper-script"
	mountPath := "/usr/local/solr-exporter-tls"
	wrapperScript := mountPath + "/launch-exporter-with-tls.sh"

	// the Prom exporter needs the keystore & truststore passwords in a Java system property, but the password
	// is stored in a file when using the mounted TLS dir approach, so we use a wrapper script around the main
	// container entry point to add these properties to JAVA_OPTS at runtime
	vol, mount := createEmptyVolumeAndMount(volName, mountPath)
	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, *vol)
	mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, *mount)

	entrypoint := mainContainer.Command[0]

	// may not have a keystore for the client cert, but must have have a truststore in that case
	kspJavaSysProp := ""
	catKsp := ""
	if opts.MountedTLSDir.KeystoreFile != "" {
		catKsp = fmt.Sprintf("ksp=\\$(cat %s)", mountedTLSKeystorePasswordPath(opts.MountedTLSDir))
		kspJavaSysProp = " -Djavax.net.ssl.keyStorePassword=\\${ksp}"
	}

	catTsp := fmt.Sprintf("tsp=\\$(cat %s)", mountedTLSTruststorePasswordPath(opts.MountedTLSDir))
	tspJavaSysProp := " -Djavax.net.ssl.trustStorePassword=\\${tsp}"

	/*
		  Create a wrapper script like:

			#!/bin/bash
			ksp=$(cat $MOUNTED_TLS_DIR/keystore-password)
			tsp=$(cat $MOUNTED_TLS_DIR/keystore-password)
			JAVA_OPTS="${JAVA_OPTS} -Djavax.net.ssl.keyStorePassword=${ksp} -Djavax.net.ssl.trustStorePassword=${tsp}"
			/opt/solr/contrib/prometheus-exporter/bin/solr-exporter $@

	*/
	writeWrapperScript := fmt.Sprintf("cat << EOF > %s\n#!/bin/bash\n%s\n%s\nJAVA_OPTS=\"\\${JAVA_OPTS}%s%s\"\n%s \\$@\nEOF\nchmod +x %s",
		wrapperScript, catKsp, catTsp, kspJavaSysProp, tspJavaSysProp, entrypoint, wrapperScript)

	createTLSWrapperScriptInitContainer := corev1.Container{
		Name:            "create-tls-wrapper-script",
		Image:           initContainerImage.ToImageName(),
		ImagePullPolicy: initContainerImage.PullPolicy,
		Command:         []string{"sh", "-c", writeWrapperScript},
		VolumeMounts:    []corev1.VolumeMount{{Name: volName, MountPath: mountPath}},
	}
	deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, createTLSWrapperScriptInitContainer)

	// Call the wrapper script to start the exporter process
	mainContainer.Command = []string{wrapperScript}
}

// Create an initContainer that generates the initdb script that exports the keystore / truststore passwords stored in
// a directory to the environment; this is only needed when using the mountedTLSDir approach
func (tls *TLSCerts) generateTLSInitdbScriptInitContainer() corev1.Container {
	// Might have a client cert too ...
	exportClientPasswords := ""
	if tls.ClientConfig != nil && tls.ClientConfig.Options.MountedTLSDir != nil {
		mountedDir := tls.ClientConfig.Options.MountedTLSDir
		if mountedDir.KeystorePasswordFile != "" {
			exportClientPasswords += exportVarFromFileInInitdbWrapperScript("SOLR_SSL_CLIENT_KEY_STORE_PASSWORD", mountedTLSKeystorePasswordPath(mountedDir))
		}
		exportClientPasswords += exportVarFromFileInInitdbWrapperScript("SOLR_SSL_CLIENT_TRUST_STORE_PASSWORD", mountedTLSTruststorePasswordPath(mountedDir))
	}

	exportServerKeystorePassword := exportVarFromFileInInitdbWrapperScript("SOLR_SSL_KEY_STORE_PASSWORD", mountedTLSKeystorePasswordPath(tls.ServerConfig.Options.MountedTLSDir))
	exportServerTruststorePassword := exportVarFromFileInInitdbWrapperScript("SOLR_SSL_TRUST_STORE_PASSWORD", mountedTLSTruststorePasswordPath(tls.ServerConfig.Options.MountedTLSDir))

	shCmd := fmt.Sprintf("echo -e \"#!/bin/bash\\n%s%s%s\"",
		exportServerKeystorePassword, exportServerTruststorePassword, exportClientPasswords)
	shCmd += " > /docker-entrypoint-initdb.d/export-tls-vars.sh"
	/*
	   Init container creates a script like:

	      #!/bin/bash

	      export SOLR_SSL_KEY_STORE_PASSWORD=`cat $MOUNTED_SERVER_TLS_DIR/keystore-password`
	      export SOLR_SSL_TRUST_STORE_PASSWORD=`cat $MOUNTED_SERVER_TLS_DIR/keystore-password`
	      export SOLR_SSL_CLIENT_KEY_STORE_PASSWORD=`cat $MOUNTED_CLIENT_TLS_DIR/keystore-password`
	      export SOLR_SSL_CLIENT_TRUST_STORE_PASSWORD=`cat $MOUNTED_CLIENT_TLS_DIR/keystore-password`

	*/

	return corev1.Container{
		Name:            "export-tls-password",
		Image:           tls.InitContainerImage.ToImageName(),
		ImagePullPolicy: tls.InitContainerImage.PullPolicy,
		Command:         []string{"sh", "-c", shCmd},
		VolumeMounts:    []corev1.VolumeMount{{Name: "initdb", MountPath: InitdbPath}},
	}
}

// Helper function for writing a line to the initdb wrapper script that exports an env var sourced from a file
func exportVarFromFileInInitdbWrapperScript(varName string, varValue string) string {
	return fmt.Sprintf("\\nexport %s=\\`cat %s\\`\\n", varName, varValue)
}

// Returns an array of Java system properties to configure the TLS certificate used by client applications to call mTLS enabled Solr pods
func (tls *TLSConfig) clientJavaOpts() []string {

	// for clients, we should always have a truststore but the keystore is optional
	javaOpts := []string{
		"-Dsolr.ssl.checkPeerName=$(SOLR_SSL_CHECK_PEER_NAME)",
		"-Djavax.net.ssl.trustStore=$(SOLR_SSL_CLIENT_TRUST_STORE)",
		"-Djavax.net.ssl.trustStoreType=PKCS12",
	}

	if tls.Options.PKCS12Secret != nil || (tls.Options.MountedTLSDir != nil && tls.Options.MountedTLSDir.KeystoreFile != "") {
		javaOpts = append(javaOpts, "-Djavax.net.ssl.keyStore=$(SOLR_SSL_CLIENT_KEY_STORE)")
		javaOpts = append(javaOpts, "-Djavax.net.ssl.keyStoreType=PKCS12")
	}

	if tls.Options.PKCS12Secret != nil {
		javaOpts = append(javaOpts, "-Djavax.net.ssl.keyStorePassword=$(SOLR_SSL_CLIENT_KEY_STORE_PASSWORD)")
	} // else for mounted dir option, the password comes from the wrapper script

	if tls.Options.PKCS12Secret != nil || tls.Options.TrustStoreSecret != nil {
		javaOpts = append(javaOpts, "-Djavax.net.ssl.trustStorePassword=$(SOLR_SSL_CLIENT_TRUST_STORE_PASSWORD)")
	} // else for mounted dir option, the password comes from the wrapper script

	return javaOpts
}

func (tls *TLSConfig) generatePkcs12InitContainer(opts *solr.SolrTLSOptions, imageName string, imagePullPolicy corev1.PullPolicy, mounts []corev1.VolumeMount) corev1.Container {
	// get the keystore password from the env for generating the keystore using openssl
	keystorePassFrom := &corev1.EnvVarSource{SecretKeyRef: opts.KeyStorePasswordSecret}
	envVars := []corev1.EnvVar{
		{
			Name:      "SOLR_SSL_KEY_STORE_PASSWORD",
			ValueFrom: keystorePassFrom,
		},
	}

	cmd := "openssl pkcs12 -export -in " + DefaultKeyStorePath + "/" + TLSCertKey + " -in " + DefaultKeyStorePath +
		"/ca.crt -inkey " + DefaultKeyStorePath + "/tls.key -out " + DefaultKeyStorePath +
		"/pkcs12/" + DefaultPkcs12KeystoreFile + " -passout pass:${SOLR_SSL_KEY_STORE_PASSWORD}"

	return corev1.Container{
		Name:                     "gen-pkcs12-keystore",
		Image:                    imageName,
		ImagePullPolicy:          imagePullPolicy,
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: "File",
		Command:                  []string{"sh", "-c", cmd},
		VolumeMounts:             mounts,
		Env:                      envVars,
	}
}

// Get TLS properties for JAVA_TOOL_OPTIONS and Java system props for configuring the secured probe command; used when
// we call a local command on the Solr pod for the probes instead of using HTTP/HTTPS
func secureProbeTLSJavaToolOpts(solrCloud *solr.SolrCloud) (tlsJavaToolOpts string, tlsJavaSysProps string) {
	if solrCloud.Spec.SolrTLS != nil {
		// prefer the mounted client cert for probes if provided
		if solrCloud.Spec.SolrClientTLS != nil && solrCloud.Spec.SolrClientTLS.MountedTLSDir != nil {
			// The keystore passwords are in a file, then we need to cat the file(s) into JAVA_TOOL_OPTIONS
			tlsJavaToolOpts += " -Djavax.net.ssl.keyStorePassword=$(cat " + mountedTLSKeystorePasswordPath(solrCloud.Spec.SolrClientTLS.MountedTLSDir) + ")"
			tlsJavaToolOpts += " -Djavax.net.ssl.trustStorePassword=$(cat " + mountedTLSTruststorePasswordPath(solrCloud.Spec.SolrClientTLS.MountedTLSDir) + ")"
		} else if solrCloud.Spec.SolrTLS.MountedTLSDir != nil {
			// If the keystore passwords are in a file, then we need to cat the file(s) into JAVA_TOOL_OPTIONS
			tlsJavaToolOpts += " -Djavax.net.ssl.keyStorePassword=$(cat " + mountedTLSKeystorePasswordPath(solrCloud.Spec.SolrTLS.MountedTLSDir) + ")"
			tlsJavaToolOpts += " -Djavax.net.ssl.trustStorePassword=$(cat " + mountedTLSTruststorePasswordPath(solrCloud.Spec.SolrTLS.MountedTLSDir) + ")"
		}
		tlsJavaSysProps = secureProbeTLSJavaSysProps(solrCloud)
	}
	return tlsJavaToolOpts, tlsJavaSysProps
}

func secureProbeTLSJavaSysProps(solrCloud *solr.SolrCloud) string {
	// probe command sends request to "localhost" so skip hostname checking during TLS handshake
	tlsJavaSysProps := "-Dsolr.ssl.checkPeerName=false"

	// use the mounted client cert if provided
	if solrCloud.Spec.SolrClientTLS != nil && solrCloud.Spec.SolrClientTLS.MountedTLSDir != nil {
		clientDir := solrCloud.Spec.SolrClientTLS.MountedTLSDir
		if clientDir.KeystoreFile != "" {
			tlsJavaSysProps += " -Djavax.net.ssl.keyStore=$SOLR_SSL_CLIENT_KEY_STORE"
		}
		tlsJavaSysProps += " -Djavax.net.ssl.trustStore=$SOLR_SSL_CLIENT_TRUST_STORE"
	} else {
		// use the server cert, either from the mounted dir or from envVars sourced from a secret
		tlsJavaSysProps += " -Djavax.net.ssl.keyStore=$SOLR_SSL_KEY_STORE"
		tlsJavaSysProps += " -Djavax.net.ssl.trustStore=$SOLR_SSL_TRUST_STORE"

		if solrCloud.Spec.SolrTLS.MountedTLSDir == nil {
			tlsJavaSysProps += " -Djavax.net.ssl.keyStorePassword=$SOLR_SSL_KEY_STORE_PASSWORD"
			tlsJavaSysProps += " -Djavax.net.ssl.trustStorePassword=$SOLR_SSL_TRUST_STORE_PASSWORD"
		} // else passwords come through JAVA_TOOL_OPTIONS via cat'ing the mounted files
	}

	return tlsJavaSysProps
}

func mountedTLSKeystorePath(tlsDir *solr.MountedTLSDirectory) string {
	return mountedTLSPath(tlsDir, tlsDir.KeystoreFile, DefaultPkcs12KeystoreFile)
}

func mountedTLSKeystorePasswordPath(tlsDir *solr.MountedTLSDirectory) string {
	return mountedTLSPath(tlsDir, tlsDir.KeystorePasswordFile, DefaultKeystorePasswordFile)
}

func mountedTLSTruststorePath(tlsDir *solr.MountedTLSDirectory) string {
	return mountedTLSPath(tlsDir, tlsDir.TruststoreFile, DefaultPkcs12TruststoreFile)
}

func mountedTLSTruststorePasswordPath(tlsDir *solr.MountedTLSDirectory) string {
	path := ""
	if tlsDir.TruststorePasswordFile != "" {
		path = mountedTLSPath(tlsDir, tlsDir.TruststorePasswordFile, "")
	} else {
		path = mountedTLSKeystorePasswordPath(tlsDir)
	}
	return path
}

func mountedTLSPath(dir *solr.MountedTLSDirectory, fileName string, defaultName string) string {
	if fileName == "" {
		fileName = defaultName
	}
	return fmt.Sprintf("%s/%s", dir.Path, fileName)
}

// Command to set the urlScheme cluster prop to "https"
func setUrlSchemeClusterPropCmd() string {
	return "solr zk ls ${ZK_CHROOT} -z ${ZK_SERVER} || solr zk mkroot ${ZK_CHROOT} -z ${ZK_SERVER}" +
		"; /opt/solr/server/scripts/cloud-scripts/zkcli.sh -zkhost ${ZK_HOST} -cmd clusterprop -name urlScheme -val https" +
		"; /opt/solr/server/scripts/cloud-scripts/zkcli.sh -zkhost ${ZK_HOST} -cmd get /clusterprops.json;"
}

// Appends additional Java system properties to the JAVA_OPTS environment variable of the main container and ensures JAVA_OPTS is the last env var
func appendJavaOptsToEnv(mainContainer *corev1.Container, additionalJavaOpts []string) {
	javaOptsValue := ""
	javaOptsAt := -1
	for i, v := range mainContainer.Env {
		if v.Name == "JAVA_OPTS" {
			javaOptsValue = v.Value
			javaOptsAt = i
			break
		}
	}
	javaOptsVar := corev1.EnvVar{Name: "JAVA_OPTS", Value: strings.TrimSpace(javaOptsValue + " " + strings.Join(additionalJavaOpts, " "))}
	if javaOptsAt == -1 {
		// no JAVA_OPTS, add it on the end
		mainContainer.Env = append(mainContainer.Env, javaOptsVar)
	} else {
		// need to move the JAVA_OPTS var to end of array, slice it out ...
		envVars := mainContainer.Env[0:javaOptsAt]
		if javaOptsAt < len(mainContainer.Env)-1 {
			envVars = append(envVars, mainContainer.Env[javaOptsAt+1:]...)
		}
		mainContainer.Env = append(envVars, javaOptsVar)
	}
}

// Utility func to create an empty dir and corresponding mount
func createEmptyVolumeAndMount(name string, mountPath string) (*corev1.Volume, *corev1.VolumeMount) {
	return &corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		&corev1.VolumeMount{Name: name, MountPath: mountPath}
}

// The Docker Solr framework allows us to run scripts from an initdb directory before the main Solr process is started
// Mount the initdb directory if it has not already been mounted by the user via custom pod options
func mountInitDbIfNeeded(stateful *appsv1.StatefulSet) {
	// Auto-TLS uses an initContainer to create a script in the initdb, so mount that if it has not already been mounted
	mainContainer := &stateful.Spec.Template.Spec.Containers[0]
	var initdbMount *corev1.VolumeMount
	for _, mount := range mainContainer.VolumeMounts {
		if mount.MountPath == InitdbPath {
			initdbMount = &mount
			break
		}
	}
	if initdbMount == nil {
		vol, mount := createEmptyVolumeAndMount("initdb", InitdbPath)
		stateful.Spec.Template.Spec.Volumes = append(stateful.Spec.Template.Spec.Volumes, *vol)
		mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, *mount)
	}
}
