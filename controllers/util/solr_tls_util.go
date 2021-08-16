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
	SolrTlsCertMd5Annotation    = "solr.apache.org/tlsCertMd5"
	DefaultKeyStorePath         = "/var/solr/tls"
	DefaultWritableKeyStorePath = "/var/solr/tls/pkcs12"
	TLSCertKey                  = "tls.crt"
	DefaultTrustStorePath       = "/var/solr/tls-truststore"
	InitdbPath                  = "/docker-entrypoint-initdb.d"
	DefaultPkcs12KeystoreFile   = "keystore.p12"
	DefaultPkcs12TruststoreFile = "truststore.p12"
	DefaultKeystorePasswordFile = "keystore-password"
)

// Holds Options from the user config as well as other config properties determined during reconciliation
// This struct is intended for internal use only and is only exposed outside the package so that the controllers can access
type TLSConfig struct {
	// TLS options provided by the user in the CRD definition
	Options *solr.SolrTLSOptions
	// Flag to indicate if we need to convert the provided keystore into the p12 format needed by Java
	NeedsPkcs12InitContainer bool
	// The MD5 hash of the TLS cert, used for restarting Solr pods after the cert updates if so desired
	CertMd5 string
	// Image used for initContainers that help configure the TLS settings
	InitContainerImage *solr.ContainerImage
}

// Enrich the config for a SolrCloud StatefulSet to enable TLS, either loaded from a secret or
// a directory on the main pod containing per-pod specific TLS files. In the latter case, the "mounted dir"
// typically comes from an external agent (such as a cert-manager extension) or CSI driver that injects the
// pod-specific TLS files using mutating web hooks
func (tls *TLSConfig) enableTLSOnSolrCloudStatefulSet(stateful *appsv1.StatefulSet) {
	// Add the SOLR_SSL_* vars to the main container's environment
	tls.enableTLSOnPodTemplate(&stateful.Spec.Template)

	// volumes and mounts for TLS when using the mounted dir option
	if tls.Options.MountedServerTLSDir != nil {
		// the TLS files come from some auto-mounted directory on the main container
		tls.mountInitDbIfNeeded(stateful)
		// use an initContainer to create the wrapper script in the initdb
		stateful.Spec.Template.Spec.InitContainers = append(stateful.Spec.Template.Spec.InitContainers, tls.generateTLSInitdbScriptInitContainer())
	}
}

// Enrich the config for a Prometheus Exporter Deployment to allow the exporter to make requests to TLS enabled Solr pods
func (tls *TLSConfig) enableTLSOnExporterDeployment(deployment *appsv1.Deployment) {
	// Add the SOLR_SSL_* vars to the main container's environment
	mainContainer := tls.enableTLSOnPodTemplate(&deployment.Spec.Template)

	// the exporter process doesn't read the SOLR_SSL_* env vars, so we need to pass them via JAVA_OPTS
	tls.appendTLSJavaOptsToEnv(mainContainer)

	// volumes and mounts for TLS when using the mounted dir option
	if tls.Options.MountedServerTLSDir != nil {
		tls.mountTLSWrapperScriptAndInitContainer(deployment)
	}
}

// Configures the TLS env vars, pod annotations, volumes, and pkcs12 initContainer on a Pod template spec
// for enabling TLS on either StatefulSet or Deployment
func (tls *TLSConfig) enableTLSOnPodTemplate(template *corev1.PodTemplateSpec) *corev1.Container {
	// Add the SOLR_SSL_* vars to the main container's environment
	mainContainer := &template.Spec.Containers[0]
	mainContainer.Env = append(mainContainer.Env, tls.envVars()...)

	if tls.Options.PKCS12Secret != nil {
		// the TLS files are mounted from a secret, setup the volumes and mounts
		template.Spec.Volumes = append(template.Spec.Volumes, tls.volumes()...)
		mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, tls.volumeMounts()...)

		// track the MD5 of the TLS cert (from secret) to trigger restarts if the cert changes
		if tls.Options.RestartOnTLSSecretUpdate && tls.CertMd5 != "" {
			if template.Annotations == nil {
				template.Annotations = make(map[string]string, 1)
			}
			template.Annotations[SolrTlsCertMd5Annotation] = tls.CertMd5
		}

		// use an initContainer to convert the keystore to p12 format if needed
		if tls.NeedsPkcs12InitContainer {
			pkcs12InitContainer := tls.generatePkcs12InitContainer(mainContainer.Image, mainContainer.ImagePullPolicy)
			template.Spec.InitContainers = append(template.Spec.InitContainers, pkcs12InitContainer)
		}
	}

	return mainContainer
}

// Get a list of volumes for the keystore and optionally a truststore loaded from a TLS secret
func (tls *TLSConfig) volumes() []corev1.Volume {
	optional := false
	defaultMode := int32(0664)
	vols := []corev1.Volume{
		{
			Name: "keystore",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tls.Options.PKCS12Secret.Name,
					DefaultMode: &defaultMode,
					Optional:    &optional,
				},
			},
		},
	}

	// if they're using a different truststore other than the keystore, but don't mount an additional volume
	// if it's just pointing at the same secret
	if tls.Options.TrustStoreSecret != nil && tls.Options.TrustStoreSecret.Name != tls.Options.PKCS12Secret.Name {
		vols = append(vols, corev1.Volume{
			Name: "truststore",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tls.Options.TrustStoreSecret.Name,
					DefaultMode: &defaultMode,
					Optional:    &optional,
				},
			},
		})
	}

	if tls.NeedsPkcs12InitContainer {
		vols = append(vols, corev1.Volume{
			Name: "pkcs12",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	return vols
}

// Get the volume mounts for the keystore and truststore
func (tls *TLSConfig) volumeMounts() []corev1.VolumeMount {
	opts := tls.Options

	mounts := []corev1.VolumeMount{
		{
			Name:      "keystore",
			ReadOnly:  true,
			MountPath: DefaultKeyStorePath,
		},
	}

	if opts.TrustStoreSecret != nil && opts.TrustStoreSecret.Name != opts.PKCS12Secret.Name {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "truststore",
			ReadOnly:  true,
			MountPath: DefaultTrustStorePath,
		})
	}

	// We need an initContainer to convert a TLS cert into the pkcs12 format Java wants (using openssl)
	// but openssl cannot write to the /var/solr/tls directory because of the way secret mounts work
	// so we need to mount an empty directory to write pkcs12 keystore into
	if tls.NeedsPkcs12InitContainer {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "pkcs12",
			ReadOnly:  false,
			MountPath: DefaultWritableKeyStorePath,
		})
	}

	return mounts
}

// Get the SOLR_SSL_* env vars for enabling TLS on Solr
func (tls *TLSConfig) envVars() []corev1.EnvVar {
	opts := tls.Options

	// Determine the correct values for the SOLR_SSL_WANT_CLIENT_AUTH and SOLR_SSL_NEED_CLIENT_AUTH vars
	wantClientAuth := "false"
	needClientAuth := "false"
	if opts.ClientAuth == solr.Need {
		needClientAuth = "true"
	} else if opts.ClientAuth == solr.Want {
		wantClientAuth = "true"
	}

	var keystoreFile string
	var passwordValueFrom *corev1.EnvVarSource
	var truststoreFile string
	var truststorePassFrom *corev1.EnvVarSource
	if opts.MountedServerTLSDir != nil {
		// TLS files are mounted by some external agent
		keystoreFile = mountedTLSKeystorePath(opts)
		truststoreFile = mountedTLSTruststorePath(opts)
	} else {
		// the keystore path depends on whether we're just loading it from the secret or whether
		// our initContainer has to generate it from the TLS secret using openssl
		// this complexity is due to the secret mount directory not being writable
		var keystorePath string
		if tls.NeedsPkcs12InitContainer {
			keystorePath = DefaultWritableKeyStorePath
		} else {
			keystorePath = DefaultKeyStorePath
		}

		keystoreFile = keystorePath + "/" + DefaultPkcs12KeystoreFile
		passwordValueFrom = &corev1.EnvVarSource{SecretKeyRef: opts.KeyStorePasswordSecret}

		// If using a truststore that is different from the keystore
		truststoreFile = keystoreFile
		truststorePassFrom = passwordValueFrom
		if opts.TrustStoreSecret != nil {
			if opts.TrustStoreSecret.Name != opts.PKCS12Secret.Name {
				// trust store is in a different secret, so will be mounted in a different dir
				truststoreFile = DefaultTrustStorePath
			} else {
				// trust store is a different key in the same secret as the keystore
				truststoreFile = DefaultKeyStorePath
			}
			truststoreFile += "/" + opts.TrustStoreSecret.Key
			if opts.TrustStorePasswordSecret != nil {
				truststorePassFrom = &corev1.EnvVarSource{SecretKeyRef: opts.TrustStorePasswordSecret}
			}
		}
	}

	envVars := []corev1.EnvVar{
		{
			Name:  "SOLR_SSL_ENABLED",
			Value: "true",
		},
		{
			Name:  "SOLR_SSL_KEY_STORE",
			Value: keystoreFile,
		},
		{
			Name:  "SOLR_SSL_TRUST_STORE",
			Value: truststoreFile,
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

	if passwordValueFrom != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "SOLR_SSL_KEY_STORE_PASSWORD", ValueFrom: passwordValueFrom})
	}

	if truststorePassFrom != nil {
		envVars = append(envVars, corev1.EnvVar{Name: "SOLR_SSL_TRUST_STORE_PASSWORD", ValueFrom: truststorePassFrom})
	}

	return envVars
}

// For the mounted dir approach, we need to customize the Prometheus exporter's entry point with a wrapper script
// that reads the keystore / truststore passwords from a file and passes them via JAVA_OPTS
func (tls *TLSConfig) mountTLSWrapperScriptAndInitContainer(deployment *appsv1.Deployment) {
	mainContainer := &deployment.Spec.Template.Spec.Containers[0]

	volName := "tls-wrapper-script"
	mountPath := "/usr/local/solr-exporter-tls"
	wrapperScript := mountPath + "/launch-exporter-with-tls.sh"

	// the Prom exporter needs the keystore & truststore passwords in a Java system property, but the password
	// is stored in a file when using the mounted TLS dir approach, so we use a wrapper script around the main
	// container entry point to add these properties to JAVA_OPTS at runtime
	vol, mount := tls.createEmptyVolumeAndMount(volName, mountPath)
	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, *vol)
	mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, *mount)

	/*
		  Create a wrapper script like:

			#!/bin/bash
			ksp=$(cat $MOUNTED_TLS_DIR/keystore-password)
			tsp=$(cat $MOUNTED_TLS_DIR/keystore-password)
			JAVA_OPTS="${JAVA_OPTS} -Djavax.net.ssl.keyStorePassword=${ksp} -Djavax.net.ssl.trustStorePassword=${tsp}"
			/opt/solr/contrib/prometheus-exporter/bin/solr-exporter $@

	*/
	entrypoint := mainContainer.Command[0]
	writeWrapperScript := fmt.Sprintf("cat << EOF > %s\n#!/bin/bash\nksp=\\$(cat %s)\ntsp=\\$(cat %s)\n"+
		"JAVA_OPTS=\"\\${JAVA_OPTS} -Djavax.net.ssl.keyStorePassword=\\${ksp} -Djavax.net.ssl.trustStorePassword=\\${tsp}\"\n%s \\$@\nEOF\nchmod +x %s",
		wrapperScript, mountedTLSKeystorePasswordPath(tls.Options), mountedTLSTruststorePasswordPath(tls.Options), entrypoint, wrapperScript)

	createTLSWrapperScriptInitContainer := corev1.Container{
		Name:            "create-tls-wrapper-script",
		Image:           tls.InitContainerImage.ToImageName(),
		ImagePullPolicy: tls.InitContainerImage.PullPolicy,
		Command:         []string{"sh", "-c", writeWrapperScript},
		VolumeMounts:    []corev1.VolumeMount{{Name: volName, MountPath: mountPath}},
	}
	deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, createTLSWrapperScriptInitContainer)

	// Call the wrapper script to start the exporter process
	mainContainer.Command = []string{wrapperScript}
}

// The Docker Solr framework allows us to run scripts from an initdb directory before the main Solr process is started
// Mount the initdb directory if it has not already been mounted by the user via custom pod options
func (tls *TLSConfig) mountInitDbIfNeeded(stateful *appsv1.StatefulSet) {
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
		vol, mount := tls.createEmptyVolumeAndMount("initdb", InitdbPath)
		stateful.Spec.Template.Spec.Volumes = append(stateful.Spec.Template.Spec.Volumes, *vol)
		mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, *mount)
	}
}

// Utility func to create an empty dir and corresponding mount
func (tls *TLSConfig) createEmptyVolumeAndMount(name string, mountPath string) (*corev1.Volume, *corev1.VolumeMount) {
	return &corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		&corev1.VolumeMount{Name: name, MountPath: mountPath}
}

// Create an initContainer that generates the initdb script that exports the keystore / truststore passwords stored in
// a directory to the environment; this is only needed when using the mountedServerTLSDir approach
func (tls *TLSConfig) generateTLSInitdbScriptInitContainer() corev1.Container {
	// run an initContainer that creates a script in the initdb that exports the
	// keystore secret from a file to the env before Solr is started
	shCmd := fmt.Sprintf("echo -e \"#!/bin/bash\\nexport SOLR_SSL_KEY_STORE_PASSWORD=\\`cat %s\\`\\nexport SOLR_SSL_TRUST_STORE_PASSWORD=\\`cat %s\\`\" > /docker-entrypoint-initdb.d/export-tls-vars.sh",
		mountedTLSKeystorePasswordPath(tls.Options), mountedTLSTruststorePasswordPath(tls.Options))

	/*
	   Init container creates a script like:

	      #!/bin/bash
	      export SOLR_SSL_KEY_STORE_PASSWORD=`cat $MOUNTED_TLS_DIR/keystore-password`
	      export SOLR_SSL_TRUST_STORE_PASSWORD=`cat $MOUNTED_TLS_DIR/keystore-password`

	*/

	return corev1.Container{
		Name:            "export-tls-password",
		Image:           tls.InitContainerImage.ToImageName(),
		ImagePullPolicy: tls.InitContainerImage.PullPolicy,
		Command:         []string{"sh", "-c", shCmd},
		VolumeMounts:    []corev1.VolumeMount{{Name: "initdb", MountPath: InitdbPath}},
	}
}

// Returns an array of Java system properties to configure the TLS certificate used by client applications to call mTLS enabled Solr pods
func (tls *TLSConfig) clientJavaOpts() []string {
	javaOpts := []string{
		"-Djavax.net.ssl.keyStore=$(SOLR_SSL_KEY_STORE)",
		"-Djavax.net.ssl.trustStore=$(SOLR_SSL_TRUST_STORE)",
		"-Djavax.net.ssl.keyStoreType=PKCS12",
		"-Djavax.net.ssl.trustStoreType=PKCS12",
		"-Dsolr.ssl.checkPeerName=$(SOLR_SSL_CHECK_PEER_NAME)",
	}

	if tls.Options.PKCS12Secret != nil {
		// the password is available from env vars set from a secret
		javaOpts = append(javaOpts, "-Djavax.net.ssl.keyStorePassword=$(SOLR_SSL_KEY_STORE_PASSWORD)")
		javaOpts = append(javaOpts, "-Djavax.net.ssl.trustStorePassword=$(SOLR_SSL_TRUST_STORE_PASSWORD)")
	} // else if mountedServerTLSDir != nil, the password is in a file and needs to be passed differently

	if tls.Options.VerifyClientHostname {
		javaOpts = append(javaOpts, "-Dsolr.jetty.ssl.verifyClientHostName=HTTPS")
	}

	return javaOpts
}

// Append TLS related Java system properties to the JAVA_OPTS environment variable of the main container
func (tls *TLSConfig) appendTLSJavaOptsToEnv(mainContainer *corev1.Container) {
	tlsJavaOpts := tls.clientJavaOpts()
	javaOptsValue := ""
	javaOptsAt := -1
	for i, v := range mainContainer.Env {
		if v.Name == "JAVA_OPTS" {
			javaOptsValue = v.Value
			javaOptsAt = i
			break
		}
	}
	javaOptsVar := corev1.EnvVar{Name: "JAVA_OPTS", Value: strings.TrimSpace(javaOptsValue + " " + strings.Join(tlsJavaOpts, " "))}
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

func (tls *TLSConfig) generatePkcs12InitContainer(imageName string, imagePullPolicy corev1.PullPolicy) corev1.Container {
	// get the keystore password from the env for generating the keystore using openssl
	passwordValueFrom := &corev1.EnvVarSource{SecretKeyRef: tls.Options.KeyStorePasswordSecret}
	envVars := []corev1.EnvVar{
		{
			Name:      "SOLR_SSL_KEY_STORE_PASSWORD",
			ValueFrom: passwordValueFrom,
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
		VolumeMounts:             tls.volumeMounts(),
		Env:                      envVars,
	}
}

// Get TLS properties for JAVA_TOOL_OPTIONS and Java system props for configuring the secured probe command; used when
// we call a local command on the Solr pod for the probes instead of using HTTP/HTTPS
func secureProbeTLSJavaToolOpts(tls *solr.SolrTLSOptions) (string, string) {
	tlsJavaToolOpts := ""
	tlsJavaSysProps := ""
	if tls != nil {
		tlsJavaSysProps = "-Djavax.net.ssl.keyStore=$SOLR_SSL_KEY_STORE -Djavax.net.ssl.trustStore=$SOLR_SSL_TRUST_STORE"

		// If the keystore passwords are in a file, then we need to cat the file(s) into JAVA_TOOL_OPTIONS
		if tls.MountedServerTLSDir != nil {
			tlsJavaToolOpts += " -Djavax.net.ssl.keyStorePassword=$(cat " + mountedTLSKeystorePasswordPath(tls) + ")"
			tlsJavaToolOpts += " -Djavax.net.ssl.trustStorePassword=$(cat " + mountedTLSTruststorePasswordPath(tls) + ")"
		} else {
			tlsJavaSysProps += " -Djavax.net.ssl.keyStorePassword=$SOLR_SSL_KEY_STORE_PASSWORD"
			tlsJavaSysProps += " -Djavax.net.ssl.trustStorePassword=$SOLR_SSL_TRUST_STORE_PASSWORD"
		}
	}
	return tlsJavaToolOpts, tlsJavaSysProps
}

func mountedTLSKeystorePath(tls *solr.SolrTLSOptions) string {
	path := ""
	if tls.MountedServerTLSDir != nil {
		path = mountedTLSPath(tls.MountedServerTLSDir, tls.MountedServerTLSDir.KeystoreFile, DefaultPkcs12KeystoreFile)
	}
	return path
}

func mountedTLSKeystorePasswordPath(tls *solr.SolrTLSOptions) string {
	path := ""
	if tls.MountedServerTLSDir != nil {
		path = mountedTLSPath(tls.MountedServerTLSDir, tls.MountedServerTLSDir.KeystorePasswordFile, DefaultKeystorePasswordFile)
	}
	return path
}

func mountedTLSTruststorePath(tls *solr.SolrTLSOptions) string {
	path := ""
	if tls.MountedServerTLSDir != nil {
		path = mountedTLSPath(tls.MountedServerTLSDir, tls.MountedServerTLSDir.TruststoreFile, DefaultPkcs12TruststoreFile)
	}
	return path
}

func mountedTLSTruststorePasswordPath(tls *solr.SolrTLSOptions) string {
	path := ""
	if tls.MountedServerTLSDir != nil {
		if tls.MountedServerTLSDir.TruststorePasswordFile != "" {
			path = mountedTLSPath(tls.MountedServerTLSDir, tls.MountedServerTLSDir.TruststorePasswordFile, "")
		} else {
			path = mountedTLSKeystorePasswordPath(tls)
		}
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
