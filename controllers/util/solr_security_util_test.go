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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	solr "github.com/apache/solr-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const probeTestPassword = "sup3r-s3cr3t-pa55"

// Stands in for 'bin/solr' on the PATH. Mimics the two things that matter for the probe command: the
// JVM echoing 'Picked up JAVA_TOOL_OPTIONS: ...' (which contains the credentials) to stderr, and the
// CLI exiting non-zero when the Solr request fails.
const fakeSolrScript = `#!/bin/sh
echo "Picked up JAVA_TOOL_OPTIONS: ${JAVA_TOOL_OPTIONS}" >&2
if [ "${FAKE_SOLR_EXIT:-0}" -ne 0 ]; then
  echo "ERROR: Parse error : HTTP ERROR 401 Bad credentials" >&2
else
  echo '{"responseHeader":{"status":0}, "status":"OK"}'
fi
exit "${FAKE_SOLR_EXIT:-0}"
`

func probeTestSolrCloud(requireAuth bool) *solr.SolrCloud {
	solrCloud := &solr.SolrCloud{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec:       solr.SolrCloudSpec{},
	}
	if requireAuth {
		solrCloud.Spec.SolrSecurity = &solr.SolrSecurityOptions{
			AuthenticationType: solr.Basic,
			ProbesRequireAuth:  true,
		}
	}
	return solrCloud
}

func probeTestHttpGetProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Scheme: corev1.URISchemeHTTP,
				Path:   "/solr" + DefaultReadinessProbePath,
				Port:   intstr.FromInt(8983),
			},
		},
	}
}

// Writes the username/password files the probe command reads, plus a fake 'solr' on the PATH.
// Returns the mount path to hand to useSecureProbe and the environment to run the probe command with.
func probeTestEnvironment(t *testing.T, solrExitCode string) (string, []string) {
	t.Helper()

	mountPath := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(mountPath, corev1.BasicAuthUsernameKey), []byte("k8s-oper"), 0600))
	assert.NoError(t, os.WriteFile(filepath.Join(mountPath, corev1.BasicAuthPasswordKey), []byte(probeTestPassword), 0600))

	binDir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(binDir, "solr"), []byte(fakeSolrScript), 0700))

	env := []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"SOLR_HOST=localhost",
		"FAKE_SOLR_EXIT=" + solrExitCode,
	}
	return mountPath, env
}

// Runs the command that useSecureProbe put on the probe, exactly as the kubelet would.
func runProbeCommand(t *testing.T, probe *corev1.Probe, env []string) (int, string) {
	t.Helper()

	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX shell available to run the probe command")
	}

	if probe.Exec == nil {
		t.Fatal("secure probes must use an exec action")
	}
	cmd := exec.Command(probe.Exec.Command[0], probe.Exec.Command[1:]...)
	cmd.Env = env
	output, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		exitErr, isExitErr := err.(*exec.ExitError)
		if !isExitErr {
			t.Fatalf("probe command failed to run: %v, output: %s", err, output)
		}
		exitCode = exitErr.ExitCode()
	}
	return exitCode, string(output)
}

// The basic auth probe command filters the credentials out of its output, but must still fail when
// Solr says no. See https://github.com/apache/solr-operator/issues/799
func TestSecureProbeWithBasicAuthReportsFailure(t *testing.T) {
	mountPath, env := probeTestEnvironment(t, "1")
	probe := probeTestHttpGetProbe()
	useSecureProbe(probeTestSolrCloud(true), probe, mountPath)

	exitCode, output := runProbeCommand(t, probe, env)

	assert.Equal(t, 1, exitCode, "probe must fail when 'solr api' fails, got output: %s", output)
	assert.NotContains(t, output, probeTestPassword, "probe output must not leak the basic auth password")
	assert.Contains(t, output, "HTTP ERROR 401 Bad credentials", "probe output should keep the error from Solr")
	assert.NotContains(t, output, "Picked up JAVA_TOOL_OPTIONS", "the JVM's JAVA_TOOL_OPTIONS line must be filtered out")
}

func TestSecureProbeWithBasicAuthReportsSuccess(t *testing.T) {
	mountPath, env := probeTestEnvironment(t, "0")
	probe := probeTestHttpGetProbe()
	useSecureProbe(probeTestSolrCloud(true), probe, mountPath)

	exitCode, output := runProbeCommand(t, probe, env)

	assert.Equal(t, 0, exitCode, "probe must succeed when 'solr api' succeeds, got output: %s", output)
	assert.NotContains(t, output, probeTestPassword, "probe output must not leak the basic auth password")
	assert.Contains(t, output, `"status":"OK"`, "probe output should keep the response from Solr")
}

// Without basic auth there is nothing to redact, so the command is a bare 'solr api' call. Its exit
// code must still be the probe's exit code.
func TestSecureProbeWithoutBasicAuthReportsFailure(t *testing.T) {
	mountPath, env := probeTestEnvironment(t, "1")
	probe := probeTestHttpGetProbe()
	useSecureProbe(probeTestSolrCloud(false), probe, mountPath)

	assert.False(t, strings.Contains(probe.Exec.Command[2], "JAVA_TOOL_OPTIONS"),
		"no JAVA_TOOL_OPTIONS handling is needed when the probes do not require auth")

	exitCode, output := runProbeCommand(t, probe, env)

	assert.Equal(t, 1, exitCode, "probe must fail when 'solr api' fails, got output: %s", output)
}
