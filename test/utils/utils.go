/*
Copyright 2025.

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

package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:golint,revive
)

const (
	clusterName         = "kind-inferno-gpu-cluster"
	prometheusHelmChart = "https://prometheus-community.github.io/helm-charts"
	monitoringNamespace = "inferno-autoscaler-monitoring"

	certmanagerVersion = "v1.16.3"
	certmanagerURLTmpl = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %s\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed with error: (%v) %s", command, err, string(output))
	}

	return string(output), nil
}

// InstallPrometheusOperator installs the prometheus Operator to be used to export the enabled metrics.
// Includes TLS certificate generation and configuration for HTTPS support.
func InstallPrometheusOperator() error {
	cmd := exec.Command("kubectl", "create", "ns", monitoringNamespace)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// Wait for namespace to be created and active
	cmd = exec.Command("kubectl", "wait", "--for=jsonpath={.status.phase}=Active", "namespace/"+monitoringNamespace, "--timeout=30s")
	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("failed to wait for namespace to be ready: %w", err)
	}

	// Generate TLS certificates for Prometheus
	if err := generateTLSCertificates(); err != nil {
		return fmt.Errorf("failed to generate TLS certificates: %w", err)
	}

	// Create Kubernetes secret for TLS certificates
	if err := createTLSCertificateSecret(); err != nil {
		return fmt.Errorf("failed to create TLS certificate secret: %w", err)
	}

	cmd = exec.Command("helm", "repo", "add", "prometheus-community", prometheusHelmChart)
	if _, err := Run(cmd); err != nil {
		return err
	}

	cmd = exec.Command("helm", "repo", "update")
	if _, err := Run(cmd); err != nil {
		return err
	}

	// Install Prometheus with TLS configuration
	cmd = exec.Command("helm", "upgrade", "-i", "kube-prometheus-stack", "prometheus-community/kube-prometheus-stack",
		"-n", monitoringNamespace,
		"-f", "hack/vllme/deploy/prometheus-operator/prometheus-tls-values.yaml")
	if _, err := Run(cmd); err != nil {
		return err
	}
	return nil
}

// UninstallPrometheusOperator uninstalls the prometheus
func UninstallPrometheusOperator() {
	cmd := exec.Command("helm", "uninstall", "kube-prometheus-stack", "-n", monitoringNamespace)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}

	cmd = exec.Command("kubectl", "delete", "ns", monitoringNamespace)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// IsPrometheusCRDsInstalled checks if any Prometheus CRDs are installed
// by verifying the existence of key CRDs related to Prometheus.
func IsPrometheusCRDsInstalled() bool {
	// List of common Prometheus CRDs
	prometheusCRDs := []string{
		"prometheuses.monitoring.coreos.com",
		"prometheusrules.monitoring.coreos.com",
		"prometheusagents.monitoring.coreos.com",
	}

	cmd := exec.Command("kubectl", "get", "crds", "-o", "custom-columns=NAME:.metadata.name")
	output, err := Run(cmd)
	if err != nil {
		return false
	}
	crdList := GetNonEmptyLines(output)
	for _, crd := range prometheusCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// generateTLSCertificates generates self-signed TLS certificates for Prometheus
func generateTLSCertificates() error {
	// Create TLS certificates directory
	cmd := exec.Command("mkdir", "-p", "hack/tls-certs")
	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("failed to create TLS certs directory: %w", err)
	}

	// Check if certificate already exists and is valid
	certFile := "hack/tls-certs/prometheus-cert.pem"
	keyFile := "hack/tls-certs/prometheus-key.pem"

	// Check if certificate is still valid (not expired)
	cmd = exec.Command("openssl", "x509", "-checkend", "86400", "-noout", "-in", certFile)
	if err := cmd.Run(); err == nil {
		// Certificate exists and is valid
		return nil
	}

	// Generate self-signed certificate
	cmd = exec.Command("openssl", "req", "-x509", "-newkey", "rsa:4096",
		"-keyout", keyFile,
		"-out", certFile,
		"-days", "3650",
		"-nodes",
		"-subj", "/CN=prometheus",
		"-addext", "subjectAltName=DNS:kube-prometheus-stack-prometheus.inferno-autoscaler-monitoring.svc.cluster.local,DNS:kube-prometheus-stack-prometheus.inferno-autoscaler-monitoring.svc,DNS:prometheus,DNS:localhost,IP:127.0.0.1")

	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("failed to generate TLS certificate: %w", err)
	}
	return nil
}

// createTLSCertificateSecret creates a Kubernetes secret for TLS certificates
func createTLSCertificateSecret() error {
	certFile := "hack/tls-certs/prometheus-cert.pem"
	keyFile := "hack/tls-certs/prometheus-key.pem"

	cmd := exec.Command("kubectl", "create", "secret", "tls", "prometheus-tls",
		"--cert="+certFile,
		"--key="+keyFile,
		"-n", monitoringNamespace)

	if _, err := Run(cmd); err != nil {
		// Secret might already exist, which is fine
		fmt.Println("TLS secret already exists or creation failed (this is usually OK)")
	}

	return nil
}

// UninstallCertManager uninstalls the cert manager
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	_, _ = Run(cmd)
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)

	_, err := Run(cmd)
	return err
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}

	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range certManagerCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// LoadImageToKindClusterWithName loads a local docker image to the kind cluster
func LoadImageToKindClusterWithName(name string, maxGPUs int) error {
	cluster, err := CheckIfClusterExistsOrCreate(maxGPUs)
	if err != nil {
		return err
	}
	kindOptions := []string{"load", "docker-image", name, "--name", cluster}
	cmd := exec.Command("kind", kindOptions...)
	_, err = Run(cmd)
	return err
}

func CheckIfClusterExistsOrCreate(maxGPUs int) (string, error) {
	// Check if the kind cluster exists
	existsCmd := exec.Command("kind", "get", "clusters")
	output, err := Run(existsCmd)
	if err != nil {
		return "", err
	}
	clusterExists := false
	clusters := GetNonEmptyLines(output)
	for _, c := range clusters {
		if c == clusterName {
			clusterExists = true
			break
		}
	}

	// Create the kind cluster if it doesn't exist
	expectedVersion := os.Getenv("K8S_EXPECTED_VERSION")
	if !clusterExists {
		scriptCmd := exec.Command("bash", "hack/create-kind-gpu-cluster.sh", "-g", fmt.Sprintf("%d", maxGPUs), "K8S_VERSION="+expectedVersion)
		if _, err := Run(scriptCmd); err != nil {
			return "", fmt.Errorf("failed to create kind cluster: %v", err)
		}
	} else {
		checkKubernetesVersion(expectedVersion)
	}

	return clusterName, nil
}

// checkKubernetesVersion verifies that the cluster is running the expected Kubernetes version
func checkKubernetesVersion(expectedVersion string) {
	By("checking Kubernetes cluster version")

	expectedVersionClean := strings.TrimPrefix(expectedVersion, "v")

	cmd := exec.Command("kubectl", "version")
	output, err := Run(cmd)
	if err != nil {
		Fail(fmt.Sprintf("Failed to get Kubernetes version: %s\n", expectedVersion))
	}

	// Extract server version
	lines := strings.Split(string(output), "\n")
	var serverVersion string
	for _, line := range lines {
		if strings.HasPrefix(line, "Server Version: v") {
			serverVersion = strings.TrimPrefix(line, "Server Version: v")
			break
		}
	}

	// Parse expected version (e.g., "1.32.0" -> major=1, minor=32)
	expectedParts := strings.Split(expectedVersionClean, ".")

	expectedMajor, err := strconv.Atoi(expectedParts[0])
	if err != nil {
		Fail(fmt.Sprintf("failed to parse expected major version: %v", err))
	}

	expectedMinor, err := strconv.Atoi(expectedParts[1])
	if err != nil {
		Fail(fmt.Sprintf("failed to parse expected minor version: %v", err))
	}

	// Parse actual server version (e.g., "1.32.0" -> major=1, minor=32)
	serverParts := strings.Split(serverVersion, ".")

	serverMajor, err := strconv.Atoi(serverParts[0])
	if err != nil {
		Fail(fmt.Sprintf("failed to parse server major version: %v", err))
	}

	serverMinor, err := strconv.Atoi(serverParts[1])
	if err != nil {
		Fail(fmt.Sprintf("failed to parse server minor version: %v", err))
	}

	fmt.Fprintf(GinkgoWriter, "Expected Kubernetes version: %s\n", expectedVersion)
	fmt.Fprintf(GinkgoWriter, "Actual Kubernetes server version: v%s\n", serverVersion)

	// Check if actual version is >= expected version
	if serverMajor < expectedMajor || (serverMajor == expectedMajor && serverMinor < expectedMinor) {
		Fail(fmt.Sprintf("Kubernetes version v%s is below required minimum %s\n", serverVersion, expectedVersion))
	}
	fmt.Fprintf(GinkgoWriter, "Kubernetes version v%s meets minimum requirement %s\n", serverVersion, expectedVersion)
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.Replace(wd, "/test/e2e", "", -1)
	return wd, nil
}

// UncommentCode searches for target in the file and remove the comment prefix
// of the target content. The target content may span multiple lines.
func UncommentCode(filename, target, prefix string) error {
	// false positive
	// nolint:gosec
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	strContent := string(content)

	idx := strings.Index(strContent, target)
	if idx < 0 {
		return fmt.Errorf("unable to find the code %s to be uncomment", target)
	}

	out := new(bytes.Buffer)
	_, err = out.Write(content[:idx])
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(bytes.NewBufferString(target))
	if !scanner.Scan() {
		return nil
	}
	for {
		_, err := out.WriteString(strings.TrimPrefix(scanner.Text(), prefix))
		if err != nil {
			return err
		}
		// Avoid writing a newline in case the previous line was the last in target.
		if !scanner.Scan() {
			break
		}
		if _, err := out.WriteString("\n"); err != nil {
			return err
		}
	}

	_, err = out.Write(content[idx+len(target):])
	if err != nil {
		return err
	}
	// false positive
	// nolint:gosec
	return os.WriteFile(filename, out.Bytes(), 0644)
}
