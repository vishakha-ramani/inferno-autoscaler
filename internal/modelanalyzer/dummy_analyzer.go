package controller

import (
	"context"
	"fmt"
	"strconv"

	llmdOptv1alpha1 "github.com/llm-d-incubation/inferno-autoscaler/api/v1alpha1"
	interfaces "github.com/llm-d-incubation/inferno-autoscaler/internal/interfaces"
)

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
) (interfaces.ModelAnalyzeResponse, error) {
	// dummy traffic shape: 40% prefill, 60% decode
	prefillRatio := 0.4
	decodeRatio := 0.6

	actualQPS := 0.0
	arrivalRate := spec.Status.CurrentAlloc.Load.ArrivalRate
	if ar, err := strconv.ParseFloat(arrivalRate, 32); err == nil {
		actualQPS = float64(ar)
	}

	requiredPrefill := actualQPS * prefillRatio
	requiredDecode := actualQPS * decodeRatio

	reason := fmt.Sprintf(
		"Split ActualQPS %.2f into prefill %.2f and decode %.2f (fixed ratio %.0f/%.0f)",
		actualQPS, requiredPrefill, requiredDecode,
		prefillRatio*100, decodeRatio*100,
	)

	allocations := make(map[string]*interfaces.ModelAcceleratorAllocation)
	for _, v := range spec.Spec.ModelProfile.Accelerators {
		allocations[v.Acc] = &interfaces.ModelAcceleratorAllocation{
			RequiredPrefillQPS: requiredPrefill,
			RequiredDecodeQPS:  requiredDecode,
			Reason:             reason,
		}
	}

	return interfaces.ModelAnalyzeResponse{
		Allocations: allocations,
	}, nil
}
