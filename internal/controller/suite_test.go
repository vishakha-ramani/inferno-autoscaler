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

package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	llmdv1alpha1 "github.com/llm-d-incubation/inferno-autoscaler/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = llmdv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

// mockPromAPI is a mock implementation of promv1.API for testing
type mockPromAPI struct{}

func (m *mockPromAPI) Query(ctx context.Context, query string, ts time.Time, opts ...promv1.Option) (model.Value, promv1.Warnings, error) {
	// Return mock data that mimics typical Prometheus responses
	vector := model.Vector{
		&model.Sample{
			Metric: model.Metric{},
			Value:  model.SampleValue(10.0), // Mock arrival rate
		},
	}
	return vector, nil, nil
}

func (m *mockPromAPI) QueryRange(ctx context.Context, query string, r promv1.Range, opts ...promv1.Option) (model.Value, promv1.Warnings, error) {
	return nil, nil, nil
}

func (m *mockPromAPI) QueryExemplars(ctx context.Context, query string, startTime, endTime time.Time) ([]promv1.ExemplarQueryResult, error) {
	return nil, nil
}

func (m *mockPromAPI) Buildinfo(ctx context.Context) (promv1.BuildinfoResult, error) {
	return promv1.BuildinfoResult{}, nil
}

func (m *mockPromAPI) Config(ctx context.Context) (promv1.ConfigResult, error) {
	return promv1.ConfigResult{}, nil
}

func (m *mockPromAPI) Flags(ctx context.Context) (promv1.FlagsResult, error) {
	return promv1.FlagsResult{}, nil
}

func (m *mockPromAPI) LabelNames(ctx context.Context, matches []string, startTime, endTime time.Time, opts ...promv1.Option) ([]string, promv1.Warnings, error) {
	return nil, nil, nil
}

func (m *mockPromAPI) LabelValues(ctx context.Context, label string, matches []string, startTime, endTime time.Time, opts ...promv1.Option) (model.LabelValues, promv1.Warnings, error) {
	return nil, nil, nil
}

func (m *mockPromAPI) Series(ctx context.Context, matches []string, startTime, endTime time.Time, opts ...promv1.Option) ([]model.LabelSet, promv1.Warnings, error) {
	return nil, nil, nil
}

func (m *mockPromAPI) GetValue(ctx context.Context, timestamp time.Time, opts ...promv1.Option) (model.Value, promv1.Warnings, error) {
	return nil, nil, nil
}

func (m *mockPromAPI) Metadata(ctx context.Context, metric, limit string) (map[string][]promv1.Metadata, error) {
	return nil, nil
}

func (m *mockPromAPI) TSDB(ctx context.Context, opts ...promv1.Option) (promv1.TSDBResult, error) {
	return promv1.TSDBResult{}, nil
}

func (m *mockPromAPI) WalReplay(ctx context.Context) (promv1.WalReplayStatus, error) {
	return promv1.WalReplayStatus{}, nil
}

func (m *mockPromAPI) Targets(ctx context.Context) (promv1.TargetsResult, error) {
	return promv1.TargetsResult{}, nil
}

func (m *mockPromAPI) TargetsMetadata(ctx context.Context, matchTarget, metric, limit string) ([]promv1.MetricMetadata, error) {
	return nil, nil
}

func (m *mockPromAPI) AlertManagers(ctx context.Context) (promv1.AlertManagersResult, error) {
	return promv1.AlertManagersResult{}, nil
}

func (m *mockPromAPI) CleanTombstones(ctx context.Context) error {
	return nil
}

func (m *mockPromAPI) DeleteSeries(ctx context.Context, matches []string, startTime, endTime time.Time) error {
	return nil
}

func (m *mockPromAPI) Snapshot(ctx context.Context, skipHead bool) (promv1.SnapshotResult, error) {
	return promv1.SnapshotResult{}, nil
}

func (m *mockPromAPI) Rules(ctx context.Context) (promv1.RulesResult, error) {
	return promv1.RulesResult{}, nil
}

func (m *mockPromAPI) Alerts(ctx context.Context) (promv1.AlertsResult, error) {
	return promv1.AlertsResult{}, nil
}

func (m *mockPromAPI) Runtimeinfo(ctx context.Context) (promv1.RuntimeinfoResult, error) {
	return promv1.RuntimeinfoResult{}, nil
}
