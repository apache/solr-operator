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
	"context"
	"crypto/sha256"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/apache/solr-operator/controllers/util/solr_api"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"math/rand"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"time"
)

const (
	SecurityJsonFile       = "security.json"
	BasicAuthMd5Annotation = "solr.apache.org/basicAuthMd5"
	DefaultProbePath       = "/admin/info/system"
)

// Utility struct holding security related config and objects resolved at runtime needed during reconciliation,
// such as the secret holding credentials the operator should use to make calls to secure Solr
type SecurityConfig struct {
	SolrSecurity      *solr.SolrSecurityOptions
	CredentialsSecret *corev1.Secret
	SecurityJson      string
	SecurityJsonSrc   *corev1.EnvVarSource
}

// Given a SolrCloud instance and an API service client, produce a SecurityConfig needed to enable Solr security
func ReconcileSecurityConfig(ctx context.Context, client *client.Client, instance *solr.SolrCloud) (*SecurityConfig, error) {
	sec := instance.Spec.SolrSecurity
	if sec.AuthenticationType == solr.Basic {
		return reconcileForBasicAuth(ctx, client, instance)
	}

	// shouldn't ever get here since the YAML would be validated against the enum before this, but keeping it here for human readers to grok the overall flow
	return nil, fmt.Errorf("%s not supported! Only 'Basic' authentication is supported by the Solr operator", sec.AuthenticationType)
}

// Reconcile the credentials and supporting config needed to make calls to Solr secured with basic auth
// Also, bootstraps an initial security.json config if not supplied by the user
// However, if users provide their own security.json, then they must also provide the basic auth secret containing
// credentials the operator should use for making calls to Solr. In other words, we don't try to infuse a new user into
// the user-provided security.json as that could get messy.
func reconcileForBasicAuth(ctx context.Context, client *client.Client, instance *solr.SolrCloud) (*SecurityConfig, error) {
	// user has the option of providing a secret with credentials the operator should use to make requests to Solr
	if instance.Spec.SolrSecurity.BasicAuthSecret != "" {
		return reconcileForBasicAuthWithUserProvidedSecret(ctx, client, instance)
	} else {
		// user didn't provide a basicAuthSecret, so it's invalid for them to provide a security.json as the operator
		// has no way of authenticating to Solr with a user provided security.json w/o also having the credentials in a secret
		if instance.Spec.SolrSecurity.BootstrapSecurityJson != nil {
			return nil, fmt.Errorf("invalid basic auth config, you must also provide the 'basicAuthSecret' when providing your own 'security.json'")
		}
		return reconcileForBasicAuthWithBootstrappedSecurityJson(ctx, client, instance)
	}
}

// Create a "bootstrap" security.json with basic auth enabled with the "admin", "solr", and "k8s" users having random passwords
func reconcileForBasicAuthWithBootstrappedSecurityJson(ctx context.Context, client *client.Client, instance *solr.SolrCloud) (*SecurityConfig, error) {
	reader := *client

	sec := instance.Spec.SolrSecurity
	security := &SecurityConfig{SolrSecurity: sec}

	// We're supplying a secret with random passwords and a default security.json
	// since we randomly generate the passwords, we need to lookup the secret first and only create if not exist
	basicAuthSecret := &corev1.Secret{}
	err := reader.Get(ctx, types.NamespacedName{Name: instance.BasicAuthSecretName(), Namespace: instance.Namespace}, basicAuthSecret)
	if err != nil && errors.IsNotFound(err) {
		authSecret, bootstrapSecret := generateBasicAuthSecretWithBootstrap(instance)

		// take ownership of these secrets since we created them
		if err := controllerutil.SetControllerReference(instance, authSecret, reader.Scheme()); err != nil {
			return nil, err
		}
		if err := controllerutil.SetControllerReference(instance, bootstrapSecret, reader.Scheme()); err != nil {
			return nil, err
		}
		err = reader.Create(ctx, authSecret)
		if err != nil {
			return nil, err
		}
		err = reader.Create(ctx, bootstrapSecret)
		if err != nil {
			return nil, err
		}

		// supply the bootstrap security.json to the initContainer via a simple BASE64 encoding env var
		security.SecurityJson = string(bootstrapSecret.Data[SecurityJsonFile])
		basicAuthSecret = authSecret
	}

	if err != nil {
		return nil, err
	}
	security.CredentialsSecret = basicAuthSecret

	if security.SecurityJson == "" {
		// the bootstrap secret already exists, so just stash the security.json needed for constructing initContainers
		bootstrapSecret := &corev1.Secret{}
		err = reader.Get(ctx, types.NamespacedName{Name: instance.SecurityBootstrapSecretName(), Namespace: instance.Namespace}, bootstrapSecret)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			} // else perhaps the user deleted it after security was bootstrapped ... this is ok but may trigger a restart on the STS
		} else {
			// stash this so we can configure the setup-zk initContainer to bootstrap the security.json in ZK
			security.SecurityJson = string(bootstrapSecret.Data[SecurityJsonFile])
			security.SecurityJsonSrc = &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: bootstrapSecret.Name}, Key: SecurityJsonFile}}
		}
	}

	return security, nil
}

// Basic auth but the user provides a secret containing credentials the operator should use to make requests to a secure Solr
func reconcileForBasicAuthWithUserProvidedSecret(ctx context.Context, client *client.Client, instance *solr.SolrCloud) (*SecurityConfig, error) {
	reader := *client

	sec := instance.Spec.SolrSecurity
	security := &SecurityConfig{SolrSecurity: sec}

	// the user supplied their own basic auth secret, make sure it exists and has the expected keys
	basicAuthSecret := &corev1.Secret{}
	if err := reader.Get(ctx, types.NamespacedName{Name: sec.BasicAuthSecret, Namespace: instance.Namespace}, basicAuthSecret); err != nil {
		return nil, err
	}

	err := ValidateBasicAuthSecret(basicAuthSecret)
	if err != nil {
		return nil, err
	}
	security.CredentialsSecret = basicAuthSecret

	// is there a user-provided security.json in a secret?
	// in this config, we don't need to enforce the user providing a security.json as they can bootstrap the security.json however they want
	if sec.BootstrapSecurityJson != nil {
		securityJson, err := loadSecurityJsonFromSecret(ctx, client, sec.BootstrapSecurityJson, instance.Namespace)
		if err != nil {
			return nil, err
		}
		security.SecurityJson = securityJson
		security.SecurityJsonSrc = &corev1.EnvVarSource{SecretKeyRef: sec.BootstrapSecurityJson}
	} // else no user-provided secret, no sweat for us

	return security, nil
}

func enableSecureProbesOnSolrCloudStatefulSet(solrCloud *solr.SolrCloud, stateful *appsv1.StatefulSet) {
	mainContainer := &stateful.Spec.Template.Spec.Containers[0]

	// if probes require auth or Solr wants client auth (mTLS), need to invoke a command on the Solr pod for the probes
	// but only Basic auth is supported for now
	mountPath := ""
	if solrCloud.Spec.SolrSecurity != nil && solrCloud.Spec.SolrSecurity.ProbesRequireAuth && solrCloud.Spec.SolrSecurity.AuthenticationType == solr.Basic {
		vol, volMount := secureProbeVolumeAndMount(solrCloud.BasicAuthSecretName())
		if vol != nil {
			stateful.Spec.Template.Spec.Volumes = append(stateful.Spec.Template.Spec.Volumes, *vol)
		}
		if volMount != nil {
			mainContainer.VolumeMounts = append(mainContainer.VolumeMounts, *volMount)
			mountPath = volMount.MountPath
		}
	}

	// update the probes if they are using HTTPGet to use an Exec to call Solr with TLS and/or Basic Auth creds
	if mainContainer.LivenessProbe.HTTPGet != nil {
		useSecureProbe(solrCloud, mainContainer.LivenessProbe, mountPath)
	}
	if mainContainer.ReadinessProbe.HTTPGet != nil {
		useSecureProbe(solrCloud, mainContainer.ReadinessProbe, mountPath)
	}
	if mainContainer.StartupProbe != nil && mainContainer.StartupProbe.HTTPGet != nil {
		useSecureProbe(solrCloud, mainContainer.StartupProbe, mountPath)
	}
}

func cmdToPutSecurityJsonInZk() string {
	scriptsDir := "/opt/solr/server/scripts/cloud-scripts"
	cmd := " ZK_SECURITY_JSON=$(%s/zkcli.sh -zkhost ${ZK_HOST} -cmd get /security.json); "
	cmd += "if [ ${#ZK_SECURITY_JSON} -lt 3 ]; then echo $SECURITY_JSON > /tmp/security.json; %s/zkcli.sh -zkhost ${ZK_HOST} -cmd putfile /security.json /tmp/security.json; echo \"put security.json in ZK\"; fi"
	return fmt.Sprintf(cmd, scriptsDir, scriptsDir)
}

// Add auth data to the supplied Context using secrets already resolved (stored in the SecurityConfig)
func (security *SecurityConfig) AddAuthToContext(ctx context.Context) (context.Context, error) {
	if security.SolrSecurity.AuthenticationType == solr.Basic {
		return contextWithBasicAuthHeader(ctx, security.CredentialsSecret), nil
	}
	return ctx, nil
}

// Similar to security.AddAuthToContext but we need to lookup the secret containing the authn credentials first
func AddAuthToContext(ctx context.Context, client *client.Client, solrCloud *solr.SolrCloud) (context.Context, error) {
	if solrCloud.Spec.SolrSecurity != nil && solrCloud.Spec.SolrSecurity.AuthenticationType == solr.Basic {
		reader := *client
		basicAuthSecret := &corev1.Secret{}
		if err := reader.Get(ctx, types.NamespacedName{Name: solrCloud.BasicAuthSecretName(), Namespace: solrCloud.Namespace}, basicAuthSecret); err != nil {
			return nil, err
		}
		return contextWithBasicAuthHeader(ctx, basicAuthSecret), nil
	}
	return ctx, nil
}

func contextWithBasicAuthHeader(ctx context.Context, basicAuthSecret *corev1.Secret) context.Context {
	creds := fmt.Sprintf("%s:%s", basicAuthSecret.Data[corev1.BasicAuthUsernameKey], basicAuthSecret.Data[corev1.BasicAuthPasswordKey])
	headerValue := "Basic " + b64.StdEncoding.EncodeToString([]byte(creds))
	return context.WithValue(ctx, solr_api.HTTP_HEADERS_CONTEXT_KEY, map[string]string{"Authorization": headerValue})
}

func ValidateBasicAuthSecret(basicAuthSecret *corev1.Secret) error {
	if basicAuthSecret.Type != corev1.SecretTypeBasicAuth {
		return fmt.Errorf("invalid secret type %v; user-provided secret %s must be of type: %v",
			basicAuthSecret.Type, basicAuthSecret.Name, corev1.SecretTypeBasicAuth)
	}
	return validateCredentialsSecretData(basicAuthSecret, solr.Basic, corev1.BasicAuthUsernameKey, corev1.BasicAuthPasswordKey)
}

func validateCredentialsSecretData(credsSecret *corev1.Secret, authType solr.AuthenticationType, userKey string, passKey string) error {
	if _, ok := credsSecret.Data[userKey]; !ok {
		return fmt.Errorf("required key '%s' not found in user-provided %s auth secret %s",
			userKey, authType, credsSecret.Name)
	}

	if _, ok := credsSecret.Data[passKey]; !ok {
		return fmt.Errorf("required key '%s' not found in user-provided %s auth secret %s",
			passKey, authType, credsSecret.Name)
	}
	return nil
}

func generateBasicAuthSecretWithBootstrap(solrCloud *solr.SolrCloud) (*corev1.Secret, *corev1.Secret) {
	securityBootstrapInfo := generateSecurityJson(solrCloud)

	labels := solrCloud.SharedLabelsWith(solrCloud.GetLabels())
	var annotations map[string]string
	basicAuthSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.BasicAuthSecretName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Data: map[string][]byte{
			corev1.BasicAuthUsernameKey: []byte(solr.DefaultBasicAuthUsername),
			corev1.BasicAuthPasswordKey: securityBootstrapInfo[solr.DefaultBasicAuthUsername],
		},
		Type: corev1.SecretTypeBasicAuth,
	}

	// this secret holds the admin and solr user credentials and the security.json needed to bootstrap Solr security
	// once the security.json is created using the setup-zk initContainer, it is not updated by the operator
	boostrapSecuritySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        solrCloud.SecurityBootstrapSecretName(),
			Namespace:   solrCloud.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Data: map[string][]byte{
			"admin":          securityBootstrapInfo["admin"],
			"solr":           securityBootstrapInfo["solr"],
			SecurityJsonFile: securityBootstrapInfo[SecurityJsonFile],
		},
		Type: corev1.SecretTypeOpaque,
	}

	return basicAuthSecret, boostrapSecuritySecret
}

func generateSecurityJson(solrCloud *solr.SolrCloud) map[string][]byte {
	blockUnknown := true

	probeRole := "\"k8s\"" // probe endpoints are secures
	if !solrCloud.Spec.SolrSecurity.ProbesRequireAuth {
		blockUnknown = false
		probeRole = "null" // a JSON null value here to allow open access
	}

	probeAuthz := ""
	for i, p := range getProbePaths(solrCloud) {
		if i > 0 {
			probeAuthz += ", "
		}
		if strings.HasPrefix(p, "/solr") {
			p = p[len("/solr"):]
		}
		probeAuthz += fmt.Sprintf("{ \"name\": \"k8s-probe-%d\", \"role\":%s, \"collection\": null, \"path\":\"%s\" }", i, probeRole, p)
	}

	// Create the user accounts for security.json with random passwords
	// hashed with random salt, just as Solr's hashing works
	username := solr.DefaultBasicAuthUsername
	users := []string{"admin", username, "solr"}
	secretData := make(map[string][]byte, len(users))
	credentials := make(map[string]string, len(users))
	for _, u := range users {
		secretData[u] = randomPassword()
		credentials[u] = solrPasswordHash(secretData[u])
	}
	credentialsJson, _ := json.Marshal(credentials)

	securityJson := fmt.Sprintf(`{
      "authentication":{
        "blockUnknown": %t,
        "class":"solr.BasicAuthPlugin",
        "credentials": %s,
        "realm":"Solr Basic Auth",
        "forwardCredentials": false
      },
      "authorization": {
        "class": "solr.RuleBasedAuthorizationPlugin",
        "user-role": {
          "admin": ["admin", "k8s"],
          "%s": ["k8s"],
          "solr": ["users", "k8s"]
        },
        "permissions": [
          %s,
          { "name": "k8s-status", "role":"k8s", "collection": null, "path":"/admin/collections" },
          { "name": "k8s-metrics", "role":"k8s", "collection": null, "path":"/admin/metrics" },
          { "name": "k8s-zk", "role":"k8s", "collection": null, "path":"/admin/zookeeper/status" },
          { "name": "k8s-ping", "role":"k8s", "collection": "*", "path":"/admin/ping" },
          { "name": "read", "role":["admin","users"] },
          { "name": "update", "role":["admin"] },
          { "name": "security-read", "role": ["admin"] },
          { "name": "security-edit", "role": ["admin"] },
          { "name": "all", "role":["admin"] }
        ]
      }
    }`, blockUnknown, credentialsJson, username, probeAuthz)

	// we need to store the security.json in the secret, otherwise we'd recompute it for every reconcile loop
	// but that doesn't work for randomized passwords ...
	secretData[SecurityJsonFile] = []byte(securityJson)

	return secretData
}

func randomPassword() []byte {
	rand.Seed(time.Now().UnixNano())
	lower := "abcdefghijklmnpqrstuvwxyz" // no 'o'
	upper := strings.ToUpper(lower)
	digits := "0123456789"
	chars := lower + upper + digits + "()[]%#@-()[]%#@-"
	pass := make([]byte, 16)
	// start with a lower char and end with an upper
	pass[0] = lower[rand.Intn(len(lower))]
	pass[len(pass)-1] = upper[rand.Intn(len(upper))]
	perm := rand.Perm(len(chars))
	for i := 1; i < len(pass)-1; i++ {
		pass[i] = chars[perm[i]]
	}
	return pass
}

func randomSaltHash() []byte {
	b := make([]byte, 32)
	rand.Read(b)
	salt := sha256.Sum256(b)
	return salt[:]
}

// this mimics the password hash generation approach used by Solr
func solrPasswordHash(passBytes []byte) string {
	// combine password with salt to create the hash
	salt := randomSaltHash()
	passHashBytes := sha256.Sum256(append(salt[:], passBytes...))
	passHashBytes = sha256.Sum256(passHashBytes[:])
	passHash := b64.StdEncoding.EncodeToString(passHashBytes[:])
	return fmt.Sprintf("%s %s", passHash, b64.StdEncoding.EncodeToString(salt))
}

// Gets a list of probe paths we need to setup authz for
func getProbePaths(solrCloud *solr.SolrCloud) []string {
	probePaths := []string{DefaultProbePath}
	probePaths = append(probePaths, GetCustomProbePaths(solrCloud)...)
	return uniqueProbePaths(probePaths)
}

func uniqueProbePaths(paths []string) []string {
	keys := make(map[string]bool)
	var set []string
	for _, name := range paths {
		if _, exists := keys[name]; !exists {
			keys[name] = true
			set = append(set, name)
		}
	}
	return set
}

func secureProbeVolumeAndMount(secretName string) (*corev1.Volume, *corev1.VolumeMount) {
	vol := &corev1.Volume{
		Name: strings.ReplaceAll(secretName, ".", "-"),
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  secretName,
				DefaultMode: &SecretReadOnlyPermissions,
			},
		},
	}
	volMount := &corev1.VolumeMount{Name: vol.Name, MountPath: fmt.Sprintf("/etc/secrets/%s", vol.Name)}
	return vol, volMount
}

func BasicAuthEnvVars(secretName string) []corev1.EnvVar {
	lor := corev1.LocalObjectReference{Name: secretName}
	usernameRef := &corev1.SecretKeySelector{LocalObjectReference: lor, Key: corev1.BasicAuthUsernameKey}
	passwordRef := &corev1.SecretKeySelector{LocalObjectReference: lor, Key: corev1.BasicAuthPasswordKey}
	return []corev1.EnvVar{
		{Name: "BASIC_AUTH_USER", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: usernameRef}},
		{Name: "BASIC_AUTH_PASS", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: passwordRef}},
	}
}

// When running with TLS and clientAuth=Need or if the probe endpoints require auth, we need to use a command instead of HTTP Get
// This function builds the custom probe command and returns any associated volume / mounts needed for the auth secrets
func useSecureProbe(solrCloud *solr.SolrCloud, probe *corev1.Probe, mountPath string) {
	// mount the secret in a file so it gets updated; env vars do not see:
	// https://kubernetes.io/docs/concepts/configuration/secret/#environment-variables-are-not-updated-after-a-secret-update
	basicAuthOption := ""
	enableBasicAuth := ""
	if solrCloud.Spec.SolrSecurity != nil && solrCloud.Spec.SolrSecurity.ProbesRequireAuth && solrCloud.Spec.SolrSecurity.AuthenticationType == solr.Basic {
		usernameFile := fmt.Sprintf("%s/%s", mountPath, corev1.BasicAuthUsernameKey)
		passwordFile := fmt.Sprintf("%s/%s", mountPath, corev1.BasicAuthPasswordKey)
		basicAuthOption = fmt.Sprintf("-Dbasicauth=$(cat %s):$(cat %s)", usernameFile, passwordFile)
		enableBasicAuth = " -Dsolr.httpclient.builder.factory=org.apache.solr.client.solrj.impl.PreemptiveBasicAuthClientBuilderFactory "
	}

	// Is TLS enabled? If so we need some additional SSL related props
	tlsJavaToolOpts, tlsJavaSysProps := secureProbeTLSJavaToolOpts(solrCloud)
	javaToolOptions := strings.TrimSpace(basicAuthOption + " " + tlsJavaToolOpts)

	// construct the probe command to invoke the SolrCLI "api" action
	//
	// and yes, this is ugly, but bin/solr doesn't expose the "api" action (as of 8.8.0) so we have to invoke java directly
	// taking some liberties on the /opt/solr path based on the official Docker image as there is no ENV var set for that path
	probeCommand := fmt.Sprintf("JAVA_TOOL_OPTIONS=\"%s\" java %s %s "+
		"-Dsolr.install.dir=\"/opt/solr\" -Dlog4j.configurationFile=\"/opt/solr/server/resources/log4j2-console.xml\" "+
		"-classpath \"/opt/solr/server/solr-webapp/webapp/WEB-INF/lib/*:/opt/solr/server/lib/ext/*:/opt/solr/server/lib/*\" "+
		"org.apache.solr.util.SolrCLI api -get %s://localhost:%d%s",
		javaToolOptions, tlsJavaSysProps, enableBasicAuth, solrCloud.UrlScheme(false), probe.HTTPGet.Port.IntVal, probe.HTTPGet.Path)
	probeCommand = regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(probeCommand), " ")

	// use an Exec instead of an HTTP GET
	probe.HTTPGet = nil
	probe.Exec = &corev1.ExecAction{Command: []string{"sh", "-c", probeCommand}}

	// minimum of 5 seconds for exec probes as they are slow to initialize
	if probe.TimeoutSeconds < 5 {
		probe.TimeoutSeconds = 5
	}
}

// Called during reconcile to load the security.json from a user-supplied secret
func loadSecurityJsonFromSecret(ctx context.Context, client *client.Client, securityJsonSecret *corev1.SecretKeySelector, ns string) (string, error) {
	sec := &corev1.Secret{}
	nn := types.NamespacedName{Name: securityJsonSecret.Name, Namespace: ns}
	reader := *client
	err := reader.Get(ctx, nn, sec)
	if err != nil {
		return "", err
	}

	securityJson, hasSecurityJson := sec.Data[securityJsonSecret.Key]
	if !hasSecurityJson {
		return "", fmt.Errorf("required key '%s' not found in the user-supplied secret %s",
			securityJsonSecret.Key, sec.Name)
	}

	return string(securityJson), nil
}
