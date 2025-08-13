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
	"net/http"
	"os"
	"os/exec"
	"time"

	v1alpha1 "github.com/llm-d-incubation/inferno-autoscaler/api/v1alpha1"
	"github.com/llm-d-incubation/inferno-autoscaler/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const controllerNamespace = "inferno-autoscaler-system"
const llmDNamespace = "llm-d-sim"

var (
	k8sClient  *kubernetes.Clientset
	crClient   client.Client
	scheme     = runtime.NewScheme()
	loadGenCmd *exec.Cmd
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

// initializeK8sClient initializes the Kubernetes client for testing
func initializeK8sClient() {
	cfg, err := func() (*rest.Config, error) {
		if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
			return clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
		return rest.InClusterConfig()
	}()
	if err != nil {
		Skip("failed to load kubeconfig: " + err.Error())
	}

	// Suppress warnings to avoid spam in test output
	cfg.WarningHandler = rest.NoWarnings{}

	k8sClient, err = kubernetes.NewForConfig(cfg)
	if err != nil {
		Skip("failed to create kubernetes client: " + err.Error())
	}

	// Initialize controller-runtime client for custom resources
	crClient, err = client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		Skip("failed to create controller-runtime client: " + err.Error())
	}
}

func startPortForwarding(service *corev1.Service, namespace string) *exec.Cmd {
	portForwardCmd := exec.Command("kubectl", "port-forward",
		fmt.Sprintf("service/%s", service.Name),
		"8000:80", "-n", namespace)
	err := portForwardCmd.Start()
	Expect(err).NotTo(HaveOccurred(), "Port-forward command should start successfully")

	// Check if the port-forward process is still running
	Eventually(func() error {
		if portForwardCmd.ProcessState != nil && portForwardCmd.ProcessState.Exited() {
			return fmt.Errorf("port-forward process exited unexpectedly with code: %d", portForwardCmd.ProcessState.ExitCode())
		}
		return nil
	}, 10*time.Second, 1*time.Second).Should(Succeed(), "Port-forward should keep running")

	return portForwardCmd
}

func startLoadGenerator() *exec.Cmd {
	// Install the load generator requirements
	requirementsCmd := exec.Command("pip", "install", "-r", "hack/vllme/vllm_emulator/requirements.txt")
	_, err := utils.Run(requirementsCmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install loadgen requirements")
	loadGenCmd = exec.Command("python",
		"hack/vllme/vllm_emulator/loadgen.py",
		"--url", "http://localhost:8000/v1",
		"--rate", "50", // 50 requests per minute to trigger scaling up
		"--content", "100", // 100 content-length per request
		"--model", "vllm")
	err = loadGenCmd.Start()
	Expect(err).NotTo(HaveOccurred(), "Failed to start load generator")

	// Check if the loadgen process is still running
	Eventually(func() error {
		if loadGenCmd.ProcessState != nil && loadGenCmd.ProcessState.Exited() {
			return fmt.Errorf("load generator exited unexpectedly with code: %d", loadGenCmd.ProcessState.ExitCode())
		}
		return nil
	}, 10*time.Second, 1*time.Second).Should(Succeed(), "Load generator should keep running")

	return loadGenCmd
}

func stopCmd(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("command or process is nil")
	}

	// Try graceful shutdown with SIGINT
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("failed to send interrupt signal: %w", err)
	}

	// Wait for graceful shutdown with timeout
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited gracefully, return any error from Wait()
		if err != nil {
			return fmt.Errorf("process exited with error: %w", err)
		}
		return nil
	case <-time.After(5 * time.Second):
		// Timeout - force kill
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill process: %w", err)
		}
		// Wait for the kill to complete
		<-done
		return nil
	}
}

// creates a vllme deployment with the specified configuration
func createVllmeDeployment(namespace, name string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":                       "vllme",
					"llm-d.ai/inferenceServing": "true",
					"llm-d.ai/model":            "ms-sim-llm-d-modelservice",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":                       "vllme",
						"llm-d.ai/inferenceServing": "true",
						"llm-d.ai/model":            "ms-sim-llm-d-modelservice",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "vllme",
							Image:           "quay.io/infernoautoscaler/vllme:0.2.1-multi-arch",
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 80},
							},
							Env: []corev1.EnvVar{
								{Name: "MODEL_NAME", Value: "default/default"},
								{Name: "DECODE_TIME", Value: "20"},
								{Name: "PREFILL_TIME", Value: "20"},
								{Name: "MODEL_SIZE", Value: "25000"},
								{Name: "KVC_PER_TOKEN", Value: "2"},
								{Name: "MAX_SEQ_LEN", Value: "2048"},
								{Name: "MEM_SIZE", Value: "80000"},
								{Name: "AVG_TOKENS", Value: "128"},
								{Name: "TOKENS_DISTRIBUTION", Value: "deterministic"},
								{Name: "MAX_BATCH_SIZE", Value: "8"},
								{Name: "REALTIME", Value: "True"},
								{Name: "MUTE_PRINT", Value: "False"},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:                    resource.MustParse("500m"),
									corev1.ResourceMemory:                 resource.MustParse("1Gi"),
									corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:                    resource.MustParse("100m"),
									corev1.ResourceMemory:                 resource.MustParse("500Mi"),
									corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyAlways,
				},
			},
		},
	}
}

// creates a service for the vllme deployment
func createVllmeService(namespace, serviceName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                       "vllme",
				"llm-d.ai/inferenceServing": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":                       "vllme",
				"llm-d.ai/inferenceServing": "true",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "vllme",
					Port:       80,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(80),
				},
			},
		},
	}
}

// creates a VariantAutoscaling resource with owner reference to deployment
func createVariantAutoscalingResource(namespace, resourceName string) *v1alpha1.VariantAutoscaling {
	return &v1alpha1.VariantAutoscaling{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: namespace,
			Labels: map[string]string{
				"inference.optimization/acceleratorName": "A100",
			},
		},
		Spec: v1alpha1.VariantAutoscalingSpec{
			ModelID: "default/default",
			SLOClassRef: v1alpha1.ConfigMapKeyRef{
				Name: "premium",
				Key:  "slo",
			},
			ModelProfile: v1alpha1.ModelProfile{
				Accelerators: []v1alpha1.AcceleratorProfile{
					{
						Acc:          "A100",
						AccCount:     1,
						Alpha:        "20.58",
						Beta:         "0.41",
						MaxBatchSize: 4,
					},
					{
						Acc:          "MI300X",
						AccCount:     1,
						Alpha:        "7.77",
						Beta:         "0.15",
						MaxBatchSize: 4,
					},
					{
						Acc:          "G2",
						AccCount:     1,
						Alpha:        "17.15",
						Beta:         "0.34",
						MaxBatchSize: 4,
					},
				},
			},
		},
	}
}

// creates a ServiceMonitor for vllme metrics collection
func createVllmeServiceMonitor() *unstructured.Unstructured {
	serviceMonitor := &unstructured.Unstructured{}
	serviceMonitor.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "ServiceMonitor",
	})
	serviceMonitor.SetName("vllme-servicemonitor")
	serviceMonitor.SetNamespace("inferno-autoscaler-monitoring")
	serviceMonitor.SetLabels(map[string]string{
		"app":     "vllme",
		"release": "kube-prometheus-stack",
	})

	spec := map[string]any{
		"selector": map[string]any{
			"matchLabels": map[string]any{
				"app": "vllme",
			},
		},
		"endpoints": []any{
			map[string]any{
				"port":     "vllme",
				"path":     "/metrics",
				"interval": "15s",
			},
		},
		"namespaceSelector": map[string]any{
			"any": true,
		},
	}
	serviceMonitor.Object["spec"] = spec

	return serviceMonitor
}

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", controllerNamespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", controllerNamespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})
		// +kubebuilder:scaffold:e2e-webhooks-checks
	})
})

var _ = Describe("Test vllme deployment with VariantAutoscaling", Ordered, func() {
	var (
		namespace  string
		deployName string
		appLabel   string
		ctx        context.Context
	)

	BeforeAll(func() {
		if os.Getenv("KUBECONFIG") == "" {
			Skip("KUBECONFIG is not set; skipping e2e test")
		}

		initializeK8sClient()

		ctx = context.Background()
		namespace = llmDNamespace
		deployName = "vllme-deployment"
		appLabel = "vllme"

		By("creating vllme deployment")
		deployment := createVllmeDeployment(namespace, deployName)
		_, err := k8sClient.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("creating vllme service")
		service := createVllmeService(namespace, "vllme-service")
		_, err = k8sClient.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("creating ServiceMonitor for vllme metrics")
		serviceMonitor := createVllmeServiceMonitor()
		err = crClient.Create(ctx, serviceMonitor)
		Expect(err).NotTo(HaveOccurred())

		By("creating VariantAutoscaling resource")
		variantAutoscaling := createVariantAutoscalingResource(namespace, deployName)
		err = crClient.Create(ctx, variantAutoscaling)
		Expect(err).NotTo(HaveOccurred())

		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	})

	It("deployment should be running", func() {
		Eventually(func() (appsv1.DeploymentStatus, error) {
			deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
			if err != nil {
				return appsv1.DeploymentStatus{}, err
			}
			return deployment.Status, nil
		}, 1*time.Minute, 10*time.Second).Should(And(
			HaveField("ReadyReplicas", BeNumerically("==", 1)),
			HaveField("Replicas", BeNumerically("==", 1)),
		))
	})

	It("deployment should have correct deployment labels", func() {
		deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("verifying deployment selector labels")
		selector := deployment.Spec.Selector.MatchLabels
		Expect(selector).To(HaveKeyWithValue("app", appLabel))
		Expect(selector).To(HaveKeyWithValue("llm-d.ai/inferenceServing", "true"))
		Expect(selector).To(HaveKeyWithValue("llm-d.ai/model", "ms-sim-llm-d-modelservice"))

		By("verifying pod template labels")
		podLabels := deployment.Spec.Template.ObjectMeta.Labels
		Expect(podLabels).To(HaveKeyWithValue("app", appLabel))
		Expect(podLabels).To(HaveKeyWithValue("llm-d.ai/inferenceServing", "true"))
		Expect(podLabels).To(HaveKeyWithValue("llm-d.ai/model", "ms-sim-llm-d-modelservice"))
	})

	It("deployment should have correct resource configuration", func() {
		deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("verifying container resource limits")
		container := deployment.Spec.Template.Spec.Containers[0]
		Expect(container.Resources.Limits).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("1")))

		By("verifying environment variables")
		var modelNameEnv, maxBatchSizeEnv *corev1.EnvVar
		for _, env := range container.Env {
			if env.Name == "MODEL_NAME" {
				modelNameEnv = &env
			}
			if env.Name == "MAX_BATCH_SIZE" {
				maxBatchSizeEnv = &env
			}
		}
		Expect(modelNameEnv).NotTo(BeNil(), "MODEL_NAME environment variable should be set")
		Expect(modelNameEnv.Value).To(Equal("default/default"))
		Expect(maxBatchSizeEnv).NotTo(BeNil(), "MAX_BATCH_SIZE environment variable should be set")
		Expect(maxBatchSizeEnv.Value).To(Equal("8"))
	})

	It("deployment should have corresponding service with correct selector", func() {
		service, err := k8sClient.CoreV1().Services(namespace).Get(ctx, "vllme-service", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("verifying service selector")
		Expect(service.Spec.Selector).To(HaveKeyWithValue("app", appLabel))
	})

	It("deployment should create pods with correct labels", func() {
		Eventually(func() ([]corev1.Pod, error) {
			podList, err := k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app=" + appLabel,
			})
			if err != nil {
				return nil, err
			}
			return podList.Items, nil
		}, 2*time.Minute, 5*time.Second).Should(HaveLen(1))

		podList, err := k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=" + appLabel,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(podList.Items).To(HaveLen(1))

		pod := podList.Items[0]
		By("verifying pod labels")
		Expect(pod.Labels).To(HaveKeyWithValue("app", appLabel))
		Expect(pod.Labels).To(HaveKeyWithValue("llm-d.ai/inferenceServing", "true"))
		Expect(pod.Labels).To(HaveKeyWithValue("llm-d.ai/model", "ms-sim-llm-d-modelservice"))
	})

	It("should have correct GPU resource requests", func() {
		deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		container := deployment.Spec.Template.Spec.Containers[0]
		Expect(container.Resources.Limits).To(HaveKeyWithValue(corev1.ResourceName("nvidia.com/gpu"), resource.MustParse("1")))
	})

	It("should have VariantAutoscaling resource created", func() {
		By("verifying VariantAutoscaling resource exists")
		variantAutoscaling := &v1alpha1.VariantAutoscaling{}
		err := crClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      deployName,
		}, variantAutoscaling)
		Expect(err).NotTo(HaveOccurred())

		By("verifying VariantAutoscaling spec")
		Expect(variantAutoscaling.Spec.ModelID).To(Equal("default/default"))
		Expect(variantAutoscaling.Spec.SLOClassRef.Name).To(Equal("premium"))
		Expect(variantAutoscaling.Spec.ModelProfile.Accelerators).To(HaveLen(3))
	})

	It("should have VariantAutoscaling with correct ownerReference to Deployment", func() {
		By("first ensuring both Deployment and VariantAutoscaling exist")
		var deployment *appsv1.Deployment
		Eventually(func(g Gomega) {
			var err error
			deployment, err = k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred(), "Deployment should exist")
			g.Expect(deployment.Status.ReadyReplicas).To(BeNumerically(">=", 1), "Deployment should have ready replicas")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("ensuring VariantAutoscaling resource exists")
		Eventually(func(g Gomega) {
			variantAutoscaling := &v1alpha1.VariantAutoscaling{}
			err := crClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      deployName,
			}, variantAutoscaling)
			g.Expect(err).NotTo(HaveOccurred(), "VariantAutoscaling should exist")
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("waiting for VariantAutoscaling to have ownerReference set by controller")
		Eventually(func(g Gomega) {
			deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to fetch Deployment")

			variantAutoscaling := &v1alpha1.VariantAutoscaling{}
			err = crClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      deployName,
			}, variantAutoscaling)
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to fetch VariantAutoscaling")

			g.Expect(variantAutoscaling.OwnerReferences).To(HaveLen(1), "VariantAutoscaling should have exactly one ownerReference")

			ownerRef := variantAutoscaling.OwnerReferences[0]
			g.Expect(ownerRef.APIVersion).To(Equal("apps/v1"), "ownerReference should have correct APIVersion")
			g.Expect(ownerRef.Kind).To(Equal("Deployment"), "ownerReference should point to a Deployment")
			g.Expect(ownerRef.Name).To(Equal(deployment.Name), "ownerReference should point to the correct Deployment name")
			g.Expect(ownerRef.UID).To(Equal(deployment.UID), "ownerReference should have the correct UID")
			g.Expect(ownerRef.Controller).NotTo(BeNil(), "ownerReference should have Controller field set")
			g.Expect(*ownerRef.Controller).To(BeTrue(), "ownerReference Controller should be true")
		}, 3*time.Minute, 2*time.Second).Should(Succeed())
	})

	It("should scale up optimized replicas when load increases", func() {
		By("verifying initial state of VariantAutoscaling")
		initialVA := &v1alpha1.VariantAutoscaling{}
		err := crClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      deployName,
		}, initialVA)
		Expect(err).NotTo(HaveOccurred())

		By("getting the service endpoint for load generation")
		service, err := k8sClient.CoreV1().Services(namespace).Get(ctx, "vllme-service", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		// Port-forward the vllme service to send requests to it
		By("setting up port-forward to the vllme service")
		portForwardCmd := startPortForwarding(service, namespace)
		defer func() {
			if err := stopCmd(portForwardCmd); err != nil {
				fmt.Printf("Warning: failed to stop port-forward command: %v\n", err)
			}
		}()

		By("waiting for port-forward to be ready")
		err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 10*time.Second, true, func(ctx context.Context) (bool, error) {
			// Try to connect to the forwarded port
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get("http://localhost:8000/v1")
			if err != nil {
				return false, nil // Retrying
			}
			defer resp.Body.Close()
			return resp.StatusCode < 500, nil // Accept any non-server error status
		})
		Expect(err).NotTo(HaveOccurred(), "Port-forward should be ready within timeout")

		By("starting load generation to create traffic")
		loadGenCmd = startLoadGenerator()
		defer func() {
			if err := stopCmd(loadGenCmd); err != nil {
				Expect(err).NotTo(HaveOccurred(), "load generator should stop gracefully")
			}
		}()

		By("waiting for load to be processed and scaling decision to be made")
		Eventually(func(g Gomega) {
			va := &v1alpha1.VariantAutoscaling{}
			err := crClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      deployName,
			}, va)
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to fetch VariantAutoscaling")

			// Verify that the optimized allocation has been computed
			g.Expect(va.Status.DesiredOptimizedAlloc.NumReplicas).To(BeNumerically(">", 0),
				"DesiredOptimizedAlloc should have calculated optimized replicas")

			// Verify that the number of replicas has scaled up
			g.Expect(va.Status.DesiredOptimizedAlloc.NumReplicas).To(BeNumerically(">", 1),
				"High load should trigger scale-up recommendation")

			deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			g.Expect(deployment.Status.Replicas).To(BeNumerically(">", 1), "Deployment should have scaled up")

		}, 10*time.Minute, 15*time.Second).Should(Succeed())

		By("verifying that the controller has updated the status")
		finalVA := &v1alpha1.VariantAutoscaling{}
		err = crClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      deployName,
		}, finalVA)
		Expect(err).NotTo(HaveOccurred())

		// Log the status for debugging
		fmt.Printf("Load Profile - Arrival Rate: %s, Avg Length: %s\n",
			finalVA.Status.CurrentAlloc.Load.ArrivalRate,
			finalVA.Status.CurrentAlloc.Load.AvgLength)
		fmt.Printf("Current Allocation - Replicas: %d, Accelerator: %s, \n",
			finalVA.Status.CurrentAlloc.NumReplicas,
			finalVA.Status.CurrentAlloc.Accelerator)
		fmt.Printf("Desired Optimized Allocation - Replicas: %d, Accelerator: %s\n",
			finalVA.Status.DesiredOptimizedAlloc.NumReplicas,
			finalVA.Status.DesiredOptimizedAlloc.Accelerator)

		deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		fmt.Printf("Current replicas for Deployment - %s: %d\n",
			deployName,
			deployment.Status.Replicas)
	})

	It("should scale down with no load", func() {
		By("waiting for scaling down decision to be made")
		Eventually(func(g Gomega) {
			va := &v1alpha1.VariantAutoscaling{}
			err := crClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      deployName,
			}, va)
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to fetch VariantAutoscaling")

			// Verify that the number of replicas has scaled down to 1
			g.Expect(va.Status.DesiredOptimizedAlloc.NumReplicas).To(BeNumerically("==", 1),
				"High load should trigger scale-up recommendation")

			deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			g.Expect(deployment.Status.Replicas).To(BeNumerically("==", 1), "Deployment should have scaled down to one replica")

		}, 10*time.Minute, 15*time.Second).Should(Succeed())

		By("verifying that the controller has updated the status")
		finalVA := &v1alpha1.VariantAutoscaling{}
		err := crClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      deployName,
		}, finalVA)
		Expect(err).NotTo(HaveOccurred())

		// Log the status for debugging
		fmt.Printf("Load Profile - Arrival Rate: %s, Avg Length: %s\n",
			finalVA.Status.CurrentAlloc.Load.ArrivalRate,
			finalVA.Status.CurrentAlloc.Load.AvgLength)
		fmt.Printf("Current Allocation - Replicas: %d, Accelerator: %s, \n",
			finalVA.Status.CurrentAlloc.NumReplicas,
			finalVA.Status.CurrentAlloc.Accelerator)
		fmt.Printf("Desired Optimized Allocation - Replicas: %d, Accelerator: %s\n",
			finalVA.Status.DesiredOptimizedAlloc.NumReplicas,
			finalVA.Status.DesiredOptimizedAlloc.Accelerator)

		deployment, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		fmt.Printf("Current replicas for Deployment - %s: %d\n",
			deployName,
			deployment.Status.Replicas)
	})

	It("should connect to Prometheus using HTTPS with TLS", func() {
		By("verifying Prometheus is accessible via HTTPS")
		Eventually(func(g Gomega) {
			// Check if Prometheus service is running with TLS
			service, err := k8sClient.CoreV1().Services("inferno-autoscaler-monitoring").Get(ctx, "kube-prometheus-stack-prometheus", metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred(), "Prometheus service should exist")
			g.Expect(service.Spec.Ports).To(ContainElement(HaveField("Port", int32(9090))), "Prometheus should be listening on port 9090")

			// Verify TLS secret exists
			secret, err := k8sClient.CoreV1().Secrets("inferno-autoscaler-monitoring").Get(ctx, "prometheus-tls", metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred(), "TLS secret should exist")
			g.Expect(secret.Data).To(HaveKey("tls.crt"), "TLS secret should contain certificate")
			g.Expect(secret.Data).To(HaveKey("tls.key"), "TLS secret should contain private key")

		}, 2*time.Minute, 10*time.Second).Should(Succeed())

		By("verifying controller can connect to Prometheus with TLS")
		Eventually(func(g Gomega) {
			// Check controller logs for successful TLS connection
			pods, err := k8sClient.CoreV1().Pods(controllerNamespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/name=inferno-autoscaler",
			})
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to list controller pods")
			g.Expect(pods.Items).NotTo(BeEmpty(), "Controller pods should exist")

			// Check logs for TLS-related messages
			pod := pods.Items[0]
			logs, err := k8sClient.CoreV1().Pods(controllerNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{
				TailLines: &[]int64{50}[0],
			}).DoRaw(ctx)
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to get controller logs")

			logString := string(logs)
			g.Expect(logString).To(ContainSubstring("TLS configuration applied to Prometheus HTTPS transport"),
				"Controller should log TLS configuration")
			g.Expect(logString).NotTo(ContainSubstring("http: server gave HTTP response to HTTPS client"),
				"Controller should not have HTTP/HTTPS mismatch errors")

		}, 3*time.Minute, 15*time.Second).Should(Succeed())
	})

	It("should handle TLS certificate verification correctly", func() {
		By("verifying TLS configuration in controller ConfigMap")
		configMap, err := k8sClient.CoreV1().ConfigMaps(controllerNamespace).Get(ctx, "inferno-autoscaler-variantautoscaling-config", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "ConfigMap should exist")

		// Verify HTTPS URL is configured
		Expect(configMap.Data["PROMETHEUS_BASE_URL"]).To(ContainSubstring("https://"),
			"Prometheus URL should use HTTPS")

		// Verify TLS settings are configured
		Expect(configMap.Data["PROMETHEUS_TLS_INSECURE_SKIP_VERIFY"]).To(Equal("true"),
			"TLS insecure skip verify should be enabled for e2e tests")

		By("verifying controller startup with TLS configuration")
		Eventually(func(g Gomega) {
			pods, err := k8sClient.CoreV1().Pods(controllerNamespace).List(ctx, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/name=inferno-autoscaler",
			})
			g.Expect(err).NotTo(HaveOccurred(), "Should be able to list controller pods")

			for _, pod := range pods.Items {
				g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning),
					fmt.Sprintf("Pod %s should be running", pod.Name))
			}
		}, 2*time.Minute, 10*time.Second).Should(Succeed())
	})

	It("should have VariantAutoscaling deleted when Deployment is deleted", func() {
		By("deleting the Deployment")
		err := k8sClient.AppsV1().Deployments(namespace).Delete(ctx, deployName, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())

		By("verifying VariantAutoscaling is deleted")
		variantAutoscaling := &v1alpha1.VariantAutoscaling{}
		err = crClient.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      deployName,
		}, variantAutoscaling)
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			return crClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      deployName,
			}, variantAutoscaling)
		}, 3*time.Minute, 2*time.Second).Should(HaveOccurred())
	})

	AfterAll(func() {
		By("deleting VariantAutoscaling resource")
		variantAutoscaling := &v1alpha1.VariantAutoscaling{
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployName,
				Namespace: namespace,
			},
		}
		err := crClient.Delete(ctx, variantAutoscaling)
		err = client.IgnoreNotFound(err)
		Expect(err).NotTo(HaveOccurred())

		By("deleting ServiceMonitor")
		serviceMonitor := &unstructured.Unstructured{}
		serviceMonitor.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "monitoring.coreos.com",
			Version: "v1",
			Kind:    "ServiceMonitor",
		})
		serviceMonitor.SetName("vllme-servicemonitor")
		serviceMonitor.SetNamespace("inferno-autoscaler-monitoring")
		err = crClient.Delete(ctx, serviceMonitor)
		err = client.IgnoreNotFound(err)
		Expect(err).NotTo(HaveOccurred())

		By("deleting vllme service")
		err = k8sClient.CoreV1().Services(namespace).Delete(ctx, "vllme-service", metav1.DeleteOptions{})
		err = client.IgnoreNotFound(err)
		Expect(err).NotTo(HaveOccurred())

		By("deleting vllme deployment")
		err = k8sClient.AppsV1().Deployments(namespace).Delete(ctx, deployName, metav1.DeleteOptions{})
		err = client.IgnoreNotFound(err)
		Expect(err).NotTo(HaveOccurred())

		By("cleaning up Prometheus operator resources")
		cmd := exec.Command("kubectl", "delete", "-f", "hack/vllme/deploy/prometheus-operator/prometheus-deploy-all-in-one.yaml", "--ignore-not-found=true")
		output, err := utils.Run(cmd)
		if err != nil {
			fmt.Printf("Prometheus cleanup output: %s\n", output)
		}
	})
})
