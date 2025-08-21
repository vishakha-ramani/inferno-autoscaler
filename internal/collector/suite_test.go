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
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

func TestCollector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Collector Suite")
}

// mockPromAPI is a mock implementation of promv1.API for testing
type mockPromAPI struct {
	queryResults map[string]model.Value
	queryErrors  map[string]error
}

func (m *mockPromAPI) Query(ctx context.Context, query string, ts time.Time, opts ...promv1.Option) (model.Value, promv1.Warnings, error) {
	if err, exists := m.queryErrors[query]; exists {
		return nil, nil, err
	}
	if val, exists := m.queryResults[query]; exists {
		return val, nil, nil
	}
	// Default return empty vector
	return model.Vector{}, nil, nil
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
