package controller

import (
	"context"
	"fmt"

	llmdOptv1alpha1 "github.com/llm-d-incubation/inferno-autoscaler/api/v1alpha1"
	interfaces "github.com/llm-d-incubation/inferno-autoscaler/internal/interfaces"
	inferno "github.com/llm-inferno/optimizer/pkg/core"
)

type ModelAnalyzer struct {
	system *inferno.System
}

func NewModelAnalyzer(system *inferno.System) *ModelAnalyzer {
	return &ModelAnalyzer{system: system}
}

func (ma *ModelAnalyzer) AnalyzeModel(ctx context.Context, variantID string) (map[string]*inferno.Allocation, error) {
	result := make(map[string]*inferno.Allocation)
	for _, accelerator := range ma.system.Accelerators() {
		acceleratorName := accelerator.Name()
		if alloc := inferno.CreateAllocation(variantID, acceleratorName); alloc != nil {
			result[acceleratorName] = alloc
		}
	}
	return result, nil
}

// SimplePrefillDecodeAnalyzer just returns prefill/decode demand.
type SimplePrefillDecodeAnalyzer struct{}

// NewSimplePrefillDecodeAnalyzer returns the analyzer.
func NewSimplePrefillDecodeAnalyzer() *SimplePrefillDecodeAnalyzer {
	return &SimplePrefillDecodeAnalyzer{}
}

// AnalyzeModel calculates required prefill/decode QPS from ActualQPS.
func (a *SimplePrefillDecodeAnalyzer) AnalyzeModel(
	ctx context.Context,
	spec llmdOptv1alpha1.VariantAutoscaling,
	metrics interfaces.MetricsSnapshot,
) (interfaces.ModelAnalyzeResponse, error) {
	// dummy traffic shape: 40% prefill, 60% decode
	prefillRatio := 0.4
	decodeRatio := 0.6

	requiredPrefill := metrics.ActualQPS * prefillRatio
	requiredDecode := metrics.ActualQPS * decodeRatio

	reason := fmt.Sprintf(
		"Split ActualQPS %.2f into prefill %.2f and decode %.2f (fixed ratio %.0f/%.0f)",
		metrics.ActualQPS, requiredPrefill, requiredDecode,
		prefillRatio*100, decodeRatio*100,
	)

	return interfaces.ModelAnalyzeResponse{
		RequiredPrefillQPS: requiredPrefill,
		RequiredDecodeQPS:  requiredDecode,
		Reason:             reason,
	}, nil
}
