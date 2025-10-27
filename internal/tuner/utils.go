package controller

import (
	"fmt"

	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/constants"
	infernoConfig "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
	tune "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/tuner"
	"github.com/llm-inferno/model-tuner/pkg/config"
)

func BuildTunerConfig(
	initState []float64,
	sloTTFT, sloITL float64,
) (*config.ConfigData, error) {

	expectedObs := []float64{sloTTFT, sloITL}

	// build config data from defaults, init state and slos
	return &config.ConfigData{
		FilterData: getDefaultFilterData(),
		ModelData: config.ModelData{
			InitState:            initState,
			PercentChange:        getDefaultPercentChange(),
			BoundedState:         true,
			MinState:             getFactoredState(initState, constants.DefaultMinStateFactor),
			MaxState:             getFactoredState(initState, constants.DefaultMaxStateFactor),
			ExpectedObservations: expectedObs,
		},
	}, nil
}

func getDefaultFilterData() config.FilterData {
	return config.FilterData{
		GammaFactor: constants.DefaultGammaFactor,
		ErrorLevel:  constants.DefaultErrorLevel,
		TPercentile: constants.DefaultTPercentile,
	}
}

func getDefaultPercentChange() []float64 {
	return []float64{
		constants.DefaultPercentChange, // alpha variance
		constants.DefaultPercentChange, // beta variance
		constants.DefaultPercentChange, // gamma variance
		constants.DefaultPercentChange, // delta variance
	}
}

func getFactoredState(initState []float64, multiplier float64) []float64 {
	multipliedNumbers := make([]float64, len(initState))
	// Iterate and multiply
	for i, num := range initState {
		multipliedNumbers[i] = num * multiplier
	}
	return multipliedNumbers
}

// ConvertAllocToEnvironment converts WVA CurrentAlloc to model-tuner Environment.
// This is the adapter between the WVA collector and the Kalman filter tuner.
func ConvertAllocToEnvironment(alloc infernoConfig.AllocationData) *tune.Environment {
	return &tune.Environment{
		Lambda:        alloc.Load.ArrivalRate,
		MaxBatchSize:  alloc.MaxBatch,
		AvgOutputToks: float32(alloc.Load.AvgOutTokens),
		AvgQueueTime:  alloc.TTFTAverage,
		AvgTokenTime:  alloc.ITLAverage,
	}
}

func findInitStateInSystemData(
	systemData *infernoConfig.SystemData,
	modelName string,
	acceleratorName string,
) ([]float64, error) {

	for _, perfData := range systemData.Spec.Models.PerfData {
		if perfData.Name == modelName && perfData.Acc == acceleratorName {
			alpha := float64(perfData.DecodeParms.Alpha)
			beta := float64(perfData.DecodeParms.Beta)
			gamma := float64(perfData.PrefillParms.Gamma)
			delta := float64(perfData.PrefillParms.Delta)

			// Validate all parameters are positive
			if alpha <= 0 || beta <= 0 || gamma <= 0 || delta <= 0 {
				return nil, fmt.Errorf("invalid parameters: alpha=%f, beta=%f, gamma=%f, delta=%f (must be positive)",
					alpha, beta, gamma, delta)
			}

			return []float64{alpha, beta, gamma, delta}, nil
		}
	}
	return nil, fmt.Errorf("model %q with accelerator %q not found in system data", modelName, acceleratorName)
}

func findSLOInSystemData(
	systemData *infernoConfig.SystemData,
	modelName string,
	serviceClassName string,
) (sloTTFT, sloITL float64, err error) {
	var svcSpecs *infernoConfig.ServiceClassSpec
	for i := range systemData.Spec.ServiceClasses.Spec {
		if systemData.Spec.ServiceClasses.Spec[i].Name == serviceClassName {
			svcSpecs = &systemData.Spec.ServiceClasses.Spec[i]
			break
		}
	}

	if svcSpecs == nil {
		return 0, 0, fmt.Errorf("service class %q not found in system data", serviceClassName)
	}

	for _, modelTarget := range svcSpecs.ModelTargets {
		if modelTarget.Model == modelName {
			sloTTFT := float64(modelTarget.SLO_TTFT)
			sloITL := float64(modelTarget.SLO_ITL)

			// Validate SLOs are positive
			if sloTTFT <= 0 || sloITL <= 0 {
				return 0, 0, fmt.Errorf("invalid SLOs for model %q: TTFT=%f, ITL=%f (must be positive)",
					modelName, sloTTFT, sloITL)
			}

			return sloTTFT, sloITL, nil
		}
	}
	return 0, 0, fmt.Errorf("model %q not found in service class %q", modelName, serviceClassName)
}

func updateModelPerfDataInSystemData(systemData *infernoConfig.SystemData, modelName, accName string, tunedResults *TunedResults) error {
	for i := range systemData.Spec.Models.PerfData {
		perfData := &systemData.Spec.Models.PerfData[i]
		if perfData.Name == modelName && perfData.Acc == accName {
			perfData.DecodeParms.Alpha = float32(tunedResults.ServiceParms.Decode.Alpha)
			perfData.DecodeParms.Beta = float32(tunedResults.ServiceParms.Decode.Beta)
			perfData.PrefillParms.Gamma = float32(tunedResults.ServiceParms.Prefill.Gamma)
			perfData.PrefillParms.Delta = float32(tunedResults.ServiceParms.Prefill.Delta)
			return nil
		}
	}
	return fmt.Errorf("model %q with accelerator %q not found in system data", modelName, accName)
}
