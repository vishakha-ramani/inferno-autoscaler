package controller

import (
	"fmt"
	"strconv"

	llmdVariantAutoscalingV1alpha1 "github.com/llm-d-incubation/workload-variant-autoscaler/api/v1alpha1"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/utils"
	infernoConfig "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
	tune "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/tuner"
)

// TuneModelPerfParams tunes performance parameters for servers that have ActivateModelTuner enabled.
// It updates SystemData with tuned parameters and updates VA status with tuned state.
func TuneModelPerfParams(
	activeVAs []llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
	systemData *infernoConfig.SystemData,
) error {
	for _, va := range activeVAs {
		// Skip if model tuner not enabled for this VA
		// TODO: What is the expected behavior of model tuner when activateModelTuner is false.
		// Should we use the init values (from the spec) or the most recently updated values?
		if !va.Spec.ActivateModelTuner {
			continue
		}

		// Find corresponding server in SystemData
		serverName := utils.FullName(va.Name, va.Namespace)
		server := findServerInSystemData(systemData, serverName)
		if server == nil {
			logger.Log.Warn("Server not found in SystemData, skipping tuning",
				"variant", va.Name,
				"namespace", va.Namespace,
				"serverName", serverName)
			continue
		}

		// Tune the params of this server
		tunedResults, err := tuneServer(&va, systemData, server)
		if err != nil {
			logger.Log.Warn("Failed to tune server, keeping original params",
				"variant", va.Name,
				"server", serverName,
				"error", err)
			continue
		}

		// Update SystemData with tuned parameters
		if err := updateModelPerfDataInSystemData(systemData, server.Model, server.CurrentAlloc.Accelerator, tunedResults); err != nil {
			logger.Log.Warn(err, "Failed to update SystemData with tuned params", "variant", va.Name)
			continue
		}

		// Update VA status (will be persisted by controller)
		if err := updateVAStatusWithTunedParams(&va, server.Model, server.CurrentAlloc.Accelerator, tunedResults); err != nil {
			logger.Log.Warn(err, "Failed to update VA status with tuned params", "variant", va.Name)
			continue
		}

		logger.Log.Info("Tuned performance parameters",
			"variant", va.Name,
			"namespace", va.Namespace,
			"alpha", tunedResults.ServiceParms.Decode.Alpha,
			"beta", tunedResults.ServiceParms.Decode.Beta,
			"gamma", tunedResults.ServiceParms.Prefill.Gamma,
			"delta", tunedResults.ServiceParms.Prefill.Delta)
	}

	return nil
}

// tuneServer tunes parameters for a single server
func tuneServer(
	va *llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
	systemData *infernoConfig.SystemData,
	server *infernoConfig.ServerSpec,
) (*tune.TunedResults, error) {
	// Check if we should skip tuning due to transient delay
	// TODO: Do we need to skip tuning based on timing

	// Create tuner for this server
	tuner, err := createTuner(va, systemData, server)
	if err != nil {
		return nil, fmt.Errorf("failed to get/create tuner: %w", err)
	}

	// Convert server's CurrentAlloc to Environment
	env := convertAllocToEnvironment(server.CurrentAlloc)

	// Validate environment has meaningful data
	if !env.Valid() {
		return nil, fmt.Errorf("invalid environment for server %s", server.Name)
	}

	// Update environment with latest metrics
	tuner.UpdateEnvironment(env)

	// Get previous NIS from VA status if it exists
	var prevNIS float64
	if hasTunedResults(va) {
		prevNISStr := va.Status.TunerPerfData.NIS
		if prevNISStr != "" {
			parsedNIS, err := strconv.ParseFloat(prevNISStr, 64)
			if err == nil {
				prevNIS = parsedNIS
			}
		}
	}

	// Run Kalman filter (predict + update)
	tunedResults, err := tuner.Run()
	if err != nil {
		// Error could mean:
		// 1. NIS validation failed (returns old state with error)
		// 2. Other failure (returns nil with error)
		if tunedResults != nil {
			// NIS validation failed, but we have previous state to use
			tunedResults.NIS = prevNIS

			logger.Log.Warn("Tuner validation failed, using previous state",
				"variant", va.Name,
				"server", server.Name,
				"NIS", tunedResults.NIS,
				"error", err)

			return tunedResults, nil
		}
		// Complete failure
		return nil, fmt.Errorf("tuner run failed: %w", err)
	}
	// Valid update
	logger.Log.Info("Tuner validation succeeded",
		"variant", va.Name,
		"server", server.Name,
		"NIS", tunedResults.NIS)
	return tunedResults, nil
}

func createTuner(
	va *llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
	systemData *infernoConfig.SystemData,
	server *infernoConfig.ServerSpec,
) (*tune.Tuner, error) {
	/*
			if the tuner doesn't exist then create a new tuner

		    ------------------------- COMMENTS --------------------------------
			Onboarding a new tuner requires initializing the Kalman Filter.
			To facilitate the initialization, we require mean value of observations (expected observations)
			on performance metrics and a current state if available, otherwise an initial state.
			The current design choice is that the current state is read from the VA status.
			If no current state is available, the filter starts with the initial state provided in the model profile field VA spec.
			This initial state is derived from offline benchmarking.
			On the other hand, it is obvious that the SLO expectation should dictate expected observations.
			------------------------------------------------------------------
	*/

	// Get state params and covariance matrix from VA status (if exists), otherwise return only the state params from VA spec
	state, covMatrix, err := getStateValsFromVA(va, systemData, server)
	if err != nil {
		return nil, fmt.Errorf("failed to get state values from the VA")
	}

	// Find SLO targets from system data
	slos, err := findSLOInSystemData(systemData, server.Model, server.Class)
	if err != nil {
		return nil, fmt.Errorf("failed to find SLO for model class pair %s, %s: %w",
			server.Model, server.Class, err)
	}

	// build tuner config from initial state and slo
	configData, err := BuildTunerConfig(state, covMatrix, slos)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	env := convertAllocToEnvironment(server.CurrentAlloc)

	// validate environment before creating tuner
	if !env.Valid() {
		return nil, fmt.Errorf("invalid environment for server %s: environment validation failed", server.Name)
	}

	tuner, err := tune.NewTuner(configData, env)
	if err != nil {
		return nil, fmt.Errorf("failed to create tuner: %w", err)
	}

	return tuner, nil
}
