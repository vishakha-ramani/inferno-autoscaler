package controller

import (
	"math"
	"testing"

	"github.com/llm-d-incubation/workload-variant-autoscaler/pkg/analyzer"
	infernoConfig "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
	"github.com/llm-d-incubation/workload-variant-autoscaler/pkg/tuner"
)

func TestBuildTunerConfig(t *testing.T) {
	initState := []float64{8.5, 2.1, 5.0, 0.11}
	slos := []float64{200.0, 20.0}

	config, err := BuildTunerConfig(initState, slos)
	if err != nil {
		t.Fatalf("BuildTunerConfig() failed: %v", err)
	}

	if config == nil {
		t.Fatal("BuildTunerConfig() returned nil config")
	}

	// Verify FilterData
	if config.FilterData.GammaFactor == 0 {
		t.Error("FilterData.GammaFactor not set")
	}
	if config.FilterData.ErrorLevel == 0 {
		t.Error("FilterData.ErrorLevel not set")
	}
	if config.FilterData.TPercentile == 0 {
		t.Error("FilterData.TPercentile not set")
	}

	// Verify ModelData
	if len(config.ModelData.InitState) != 4 {
		t.Errorf("Expected InitState length 4, got %d", len(config.ModelData.InitState))
	}

	if len(config.ModelData.ExpectedObservations) != 2 {
		t.Errorf("Expected ExpectedObservations length 2, got %d", len(config.ModelData.ExpectedObservations))
	}

	if !config.ModelData.BoundedState {
		t.Error("BoundedState should be true")
	}

	// Verify bounds exist
	if len(config.ModelData.MinState) != 4 {
		t.Errorf("Expected MinState length 4, got %d", len(config.ModelData.MinState))
	}
	if len(config.ModelData.MaxState) != 4 {
		t.Errorf("Expected MaxState length 4, got %d", len(config.ModelData.MaxState))
	}

	// Verify PercentChange
	if len(config.ModelData.PercentChange) != 4 {
		t.Errorf("Expected PercentChange length 4, got %d", len(config.ModelData.PercentChange))
	}
}

func TestBuildTunerConfigEmptySlo(t *testing.T) {
	initState := []float64{8.5, 2.1, 5.0, 0.11}
	slos := []float64{} // empty slos

	_, err := BuildTunerConfig(initState, slos)
	if err == nil {
		t.Fatalf("BuildTunerConfig() should have failed for empty slos: %v", err)
	}
}

func TestBuildTunerConfigEmptyInitState(t *testing.T) {
	initState := []float64{} // empty init state
	slos := []float64{200.0, 20.0}

	_, err := BuildTunerConfig(initState, slos)
	if err == nil {
		t.Fatalf("BuildTunerConfig() should have failed for empty init state: %v", err)
	}
}

func TestConvertAllocToEnvironment(t *testing.T) {
	alloc := infernoConfig.AllocationData{
		NumReplicas: 2,
		MaxBatch:    8,
		Accelerator: "A100",
		Load: infernoConfig.ServerLoadSpec{
			ArrivalRate:  60.0, // req/min total
			AvgInTokens:  512,
			AvgOutTokens: 128,
		},
		TTFTAverage: 186.7,
		ITLAverage:  14.9,
	}

	env := ConvertAllocToEnvironment(alloc)

	if env == nil {
		t.Fatal("ConvertAllocToEnvironment() returned nil")
	}

	// Verify rate per replica calculation (60 / 2 = 30)
	expectedLambda := float32(30.0)
	if env.Lambda != expectedLambda {
		t.Errorf("Expected Lambda %.1f, got %.1f", expectedLambda, env.Lambda)
	}

	if env.AvgInputToks != alloc.Load.AvgInTokens {
		t.Errorf("AvgInputToks mismatch: expected %d, got %d", alloc.Load.AvgInTokens, env.AvgInputToks)
	}

	if env.AvgOutputToks != alloc.Load.AvgOutTokens {
		t.Errorf("AvgOutputToks mismatch: expected %d, got %d", alloc.Load.AvgOutTokens, env.AvgOutputToks)
	}

	if env.MaxBatchSize != alloc.MaxBatch {
		t.Errorf("MaxBatchSize mismatch: expected %d, got %d", alloc.MaxBatch, env.MaxBatchSize)
	}

	if env.AvgTTFT != alloc.TTFTAverage {
		t.Errorf("AvgTTFT mismatch: expected %.1f, got %.1f", alloc.TTFTAverage, env.AvgTTFT)
	}

	if env.AvgITL != alloc.ITLAverage {
		t.Errorf("AvgITL mismatch: expected %.1f, got %.1f", alloc.ITLAverage, env.AvgITL)
	}

	// Verify environment is valid
	if !env.Valid() {
		t.Error("Converted environment should be valid")
	}
}

func TestConvertAllocToEnvironment_SingleReplica(t *testing.T) {
	alloc := infernoConfig.AllocationData{
		NumReplicas: 1,
		Load: infernoConfig.ServerLoadSpec{
			ArrivalRate: 60.0,
		},
	}

	env := ConvertAllocToEnvironment(alloc)

	// With 1 replica, lambda should equal arrival rate
	if env.Lambda != 60.0 {
		t.Errorf("Expected Lambda 60.0, got %.1f", env.Lambda)
	}
}

func TestFindInitStateInSystemData(t *testing.T) {
	systemData := &infernoConfig.SystemData{
		Spec: infernoConfig.SystemSpec{
			Models: infernoConfig.ModelData{
				PerfData: []infernoConfig.ModelAcceleratorPerfData{
					{
						Name: "llama-7b",
						Acc:  "A100",
						DecodeParms: infernoConfig.DecodeParms{
							Alpha: 8.5,
							Beta:  2.1,
						},
						PrefillParms: infernoConfig.PrefillParms{
							Gamma: 5.0,
							Delta: 0.11,
						},
					},
					{
						Name: "llama-13b",
						Acc:  "A100",
						DecodeParms: infernoConfig.DecodeParms{
							Alpha: 10.0,
							Beta:  3.0,
						},
						PrefillParms: infernoConfig.PrefillParms{
							Gamma: 7.0,
							Delta: 0.15,
						},
					},
				},
			},
		},
	}

	// Test finding existing model+accelerator
	initState, err := findInitStateInSystemData(systemData, "llama-7b", "A100")
	if err != nil {
		t.Fatalf("findInitStateInSystemData() failed: %v", err)
	}

	if len(initState) != 4 {
		t.Errorf("Expected initState length 4, got %d", len(initState))
	}

	expectedState := []float64{8.5, 2.1, 5.0, 0.11}
	for i, expected := range expectedState {
		diff := math.Abs(initState[i] - expected)
		if diff > 1e-6 {
			t.Errorf("initState[%d]: expected %.2f, got %.2f", i, expected, initState[i])
		}
	}

	// Test model not found
	_, err = findInitStateInSystemData(systemData, "nonexistent", "A100")
	if err == nil {
		t.Error("findInitStateInSystemData() should fail for nonexistent model")
	}

	// Test accelerator not found
	_, err = findInitStateInSystemData(systemData, "llama-7b", "V100")
	if err == nil {
		t.Error("findInitStateInSystemData() should fail for nonexistent accelerator")
	}
}

func TestFindInitStateInSystemData_InvalidParameters(t *testing.T) {
	systemData := &infernoConfig.SystemData{
		Spec: infernoConfig.SystemSpec{
			Models: infernoConfig.ModelData{
				PerfData: []infernoConfig.ModelAcceleratorPerfData{
					{
						Name: "invalid-model",
						Acc:  "A100",
						DecodeParms: infernoConfig.DecodeParms{
							Alpha: -1.0, // Invalid: negative
							Beta:  2.1,
						},
						PrefillParms: infernoConfig.PrefillParms{
							Gamma: 5.0,
							Delta: 0.11,
						},
					},
				},
			},
		},
	}

	_, err := findInitStateInSystemData(systemData, "invalid-model", "A100")
	if err == nil {
		t.Error("findInitStateInSystemData() should fail for invalid (negative) parameters")
	}
}

func TestFindSLOInSystemData(t *testing.T) {
	systemData := &infernoConfig.SystemData{
		Spec: infernoConfig.SystemSpec{
			ServiceClasses: infernoConfig.ServiceClassData{
				Spec: []infernoConfig.ServiceClassSpec{
					{
						Name: "premium",
						ModelTargets: []infernoConfig.ModelTarget{
							{
								Model:    "llama-7b",
								SLO_TTFT: 150.0,
								SLO_ITL:  15.0,
							},
							{
								Model:    "llama-13b",
								SLO_TTFT: 200.0,
								SLO_ITL:  20.0,
							},
						},
					},
					{
						Name: "standard",
						ModelTargets: []infernoConfig.ModelTarget{
							{
								Model:    "llama-7b",
								SLO_TTFT: 300.0,
								SLO_ITL:  30.0,
							},
						},
					},
				},
			},
		},
	}

	// Test finding existing model+class
	slos, err := findSLOInSystemData(systemData, "llama-7b", "premium")
	if err != nil {
		t.Fatalf("findSLOInSystemData() failed: %v", err)
	}

	if len(slos) != 2 {
		t.Errorf("Expected slos length 2, got %d", len(slos))
	}

	if slos[0] != 150.0 || slos[1] != 15.0 {
		t.Errorf("Expected SLOs [150.0, 15.0], got [%.1f, %.1f]", slos[0], slos[1])
	}

	// Test different class
	slos, err = findSLOInSystemData(systemData, "llama-7b", "standard")
	if err != nil {
		t.Fatalf("findSLOInSystemData() failed for standard class: %v", err)
	}

	if slos[0] != 300.0 || slos[1] != 30.0 {
		t.Errorf("Expected SLOs [300.0, 30.0], got [%.1f, %.1f]", slos[0], slos[1])
	}

	// Test service class not found
	_, err = findSLOInSystemData(systemData, "llama-7b", "nonexistent")
	if err == nil {
		t.Error("findSLOInSystemData() should fail for nonexistent service class")
	}

	// Test model not found in class
	_, err = findSLOInSystemData(systemData, "nonexistent", "premium")
	if err == nil {
		t.Error("findSLOInSystemData() should fail for nonexistent model in service class")
	}
}

func TestFindSLOInSystemData_InvalidSLOs(t *testing.T) {
	systemData := &infernoConfig.SystemData{
		Spec: infernoConfig.SystemSpec{
			ServiceClasses: infernoConfig.ServiceClassData{
				Spec: []infernoConfig.ServiceClassSpec{
					{
						Name: "invalid",
						ModelTargets: []infernoConfig.ModelTarget{
							{
								Model:    "test-model",
								SLO_TTFT: -100.0, // Invalid: negative
								SLO_ITL:  15.0,
							},
						},
					},
				},
			},
		},
	}

	_, err := findSLOInSystemData(systemData, "test-model", "invalid")
	if err == nil {
		t.Error("findSLOInSystemData() should fail for invalid (negative) SLOs")
	}
}

func TestUpdateModelPerfDataInSystemData(t *testing.T) {
	systemData := &infernoConfig.SystemData{
		Spec: infernoConfig.SystemSpec{
			Models: infernoConfig.ModelData{
				PerfData: []infernoConfig.ModelAcceleratorPerfData{
					{
						Name: "llama-7b",
						Acc:  "A100",
						DecodeParms: infernoConfig.DecodeParms{
							Alpha: 8.5,
							Beta:  2.1,
						},
						PrefillParms: infernoConfig.PrefillParms{
							Gamma: 5.0,
							Delta: 0.11,
						},
					},
				},
			},
		},
	}

	tunedResults := &tuner.TunedResults{
		ServiceParms: &analyzer.ServiceParms{
			Decode: &analyzer.DecodeParms{
				Alpha: 9.0,
				Beta:  2.5,
			},
			Prefill: &analyzer.PrefillParms{
				Gamma: 6.0,
				Delta: 0.12,
			},
		},
	}

	// Test updating existing model+accelerator
	err := updateModelPerfDataInSystemData(systemData, "llama-7b", "A100", tunedResults)
	if err != nil {
		t.Fatalf("updateModelPerfDataInSystemData() failed: %v", err)
	}

	// Verify parameters were updated
	perfData := systemData.Spec.Models.PerfData[0]
	if perfData.DecodeParms.Alpha != 9.0 {
		t.Errorf("Alpha not updated: expected 9.0, got %.1f", perfData.DecodeParms.Alpha)
	}
	if perfData.DecodeParms.Beta != 2.5 {
		t.Errorf("Beta not updated: expected 2.5, got %.1f", perfData.DecodeParms.Beta)
	}
	if perfData.PrefillParms.Gamma != 6.0 {
		t.Errorf("Gamma not updated: expected 6.0, got %.1f", perfData.PrefillParms.Gamma)
	}
	if perfData.PrefillParms.Delta != 0.12 {
		t.Errorf("Delta not updated: expected 0.12, got %.2f", perfData.PrefillParms.Delta)
	}

	// Test model not found
	err = updateModelPerfDataInSystemData(systemData, "nonexistent", "A100", tunedResults)
	if err == nil {
		t.Error("updateModelPerfDataInSystemData() should fail for nonexistent model")
	}

	// Test accelerator not found
	err = updateModelPerfDataInSystemData(systemData, "llama-7b", "V100", tunedResults)
	if err == nil {
		t.Error("updateModelPerfDataInSystemData() should fail for nonexistent accelerator")
	}
}

func TestGetFactoredState(t *testing.T) {
	initState := []float64{8.5, 2.1, 5.0, 0.11}
	multiplier := 2.0

	result := getFactoredState(initState, multiplier)

	if len(result) != len(initState) {
		t.Errorf("Result length mismatch: expected %d, got %d", len(initState), len(result))
	}

	for i, val := range initState {
		expected := val * multiplier
		if result[i] != expected {
			t.Errorf("result[%d]: expected %.2f, got %.2f", i, expected, result[i])
		}
	}
}

func TestGetFactoredState_MinMax(t *testing.T) {
	initState := []float64{10.0, 5.0, 8.0, 0.1}

	// Test min state (0.1x)
	minState := getFactoredState(initState, 0.1)
	expectedMin := []float64{1.0, 0.5, 0.8, 0.01}
	for i := range expectedMin {
		diff := math.Abs(minState[i] - expectedMin[i])
		if diff > 1e-6 {
			t.Errorf("minState[%d]: expected %4f, got %4f", i, expectedMin[i], minState[i])
		}
	}

	// Test max state (10x)
	maxState := getFactoredState(initState, 10.0)
	expectedMax := []float64{100.0, 50.0, 80.0, 1.0}
	for i := range expectedMax {
		diff := math.Abs(maxState[i] - expectedMax[i])
		if diff > 1e-6 {
			t.Errorf("maxState[%d]: expected %4f, got %4f", i, expectedMax[i], maxState[i])
		}
	}
}
