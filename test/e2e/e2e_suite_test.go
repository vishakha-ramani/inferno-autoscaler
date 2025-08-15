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

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/llm-d-incubation/inferno-autoscaler/test/utils"

	ctrlutils "github.com/llm-d-incubation/inferno-autoscaler/internal/utils"
)

var (
	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if CertManager is already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false

	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "quay.io/infernoautoscaler/inferno-controller:0.0.1-test"
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purposed to be used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting inferno-autoscaler integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the manager(Operator) image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	// TODO(user): If you want to change the e2e test vendor from Kind, ensure the image is
	// built and available before running the tests. Also, remove the following block.
	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage, maximumAvailableGPUs)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	initializeK8sClient()

	By("creating Inferno-autoscaler-system namespace")
	cmd = exec.Command("kubectl", "create", "ns", controllerNamespace)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

	By("creating llm-d-sim namespace")
	cmd = exec.Command("kubectl", "create", "ns", llmDNamespace)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create namespace", llmDNamespace)

	By("installing Prometheus")
	Expect(utils.InstallPrometheusOperator()).To(Succeed(), "Failed to install Prometheus Operator")

	By("installing CRDs")
	cmd = exec.Command("make", "install")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("creating the serviceclass ConfigMap")
	serviceclassConfigMap := ctrlutils.CreateServiceClassConfigMap(controllerNamespace)
	_, err = k8sClient.CoreV1().ConfigMaps(controllerNamespace).Create(context.Background(), serviceclassConfigMap, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create serviceclass ConfigMap")

	By("creating the accelerator unitcost ConfigMap")
	acceleratorConfigMap := ctrlutils.CreateAcceleratorUnitCostConfigMap(controllerNamespace)
	_, err = k8sClient.CoreV1().ConfigMaps(controllerNamespace).Create(context.Background(), acceleratorConfigMap, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create accelerator unitcost ConfigMap")

	By("creating the Prometheus configuration ConfigMap with TLS settings")
	prometheusConfigMap := createPrometheusConfigMapWithTLS(controllerNamespace)
	_, err = k8sClient.CoreV1().ConfigMaps(controllerNamespace).Create(context.Background(), prometheusConfigMap, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred(), "Failed to create Prometheus configuration ConfigMap")

	cmd = exec.Command("kubectl", "apply", "-f", "hack/vllme/deploy/prometheus-operator/prometheus-deploy-all-in-one.yaml")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy Prometheus resources")

	By("deploying the controller-manager")
	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for its presence before execution.
	// Setup CertManager before the suite if not skipped and if not already installed
	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = utils.IsCertManagerCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
			Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
		}
	}
})

var _ = AfterSuite(func() {
	// Teardown CertManager after the suite if not skipped and if it was not already installed
	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}

	// Destroy the Kind cluster
	cmd := exec.Command("bash", "hack/destroy-kind-cluster.sh")
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to destroy Kind cluster")
})

// createPrometheusConfigMapWithTLS creates a ConfigMap with Prometheus configuration including TLS settings
func createPrometheusConfigMapWithTLS(namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inferno-autoscaler-variantautoscaling-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			// Prometheus configuration with HTTPS and TLS
			"PROMETHEUS_BASE_URL": "https://kube-prometheus-stack-prometheus.inferno-autoscaler-monitoring.svc.cluster.local:9090",

			// TLS configuration for e2e tests (using self-signed certificates)
			"PROMETHEUS_TLS_INSECURE_SKIP_VERIFY": "true",

			// Optimization configuration
			"GLOBAL_OPT_INTERVAL": "60s",
			"GLOBAL_OPT_TRIGGER":  "false",
		},
	}
}
