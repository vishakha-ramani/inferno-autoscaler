package controller

import (
	"fmt"
	"time"

	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/constants"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	infernoConfig "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
	tune "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/tuner"
)

// TunerManager manages Kalman filter tuners for all VariantAutoscalings.
// Assumption: Server environments are assumed to be prefill and decode.
type TunerManager struct {
	// Map of server (variant) name to its tuner
	tuners map[string]*tune.Tuner
	// whether tuner manager is enabled
	enabled bool
}

func NewTunerManager() *TunerManager {
	return &TunerManager{
		tuners:  make(map[string]*tune.Tuner),
		enabled: true,
	}
}

func (tm *TunerManager) IsEnabled() bool {
	return tm.enabled
}

func (tm *TunerManager) Enable() {
	tm.enabled = true
}

func (tm *TunerManager) Disable() {
	tm.enabled = false
}

// TuneModelPerfParams tunes performance model parameters for all servers in SystemData
func (tm *TunerManager) TuneModelPerfParams(systemData *infernoConfig.SystemData) error {
	// tune model tuner for each server
	for i := range systemData.Spec.Servers.Spec {
		server := &systemData.Spec.Servers.Spec[i]
		if err := tm.tuneServer(systemData, server); err != nil {
			logger.Log.Warn("Failed to tune server, keeping original params",
				"server", server.Name,
				"error", err)
			continue
		}
	}
	return nil
}

// tuneServer tunes parameters for a single server and updates SystemData.
func (tm *TunerManager) tuneServer(systemData *infernoConfig.SystemData, server *infernoConfig.ServerSpec) error {
	// Check if we should skip tuning due to transient delay
	// TODO: Do we need to skip tuning based on timing
	skip, err := tm.skipIfInTransition(server)
	if err != nil {
		logger.Log.Info("Failed to check transition state: %w", err)
	}
	if skip {
		logger.Log.Info("Skipping tuning due to transient delay", "server", server.Name)
		return nil
	}
	// Get or create tuner for this server
	tuner, err := tm.getOrCreateTuner(systemData, server)
	if err != nil {
		return fmt.Errorf("failed to get/create tuner: %w", err)
	}

	// Convert server's CurrentAlloc to Environment
	env := ConvertAllocToEnvironment(server.CurrentAlloc)

	// Validate environment has meaningful data
	if !env.Valid() {
		return fmt.Errorf("invalid environment for server %s", server.Name)
	}

	// Update environment with latest metrics
	err = tuner.UpdateEnvironment(env)
	if err != nil {
		return fmt.Errorf("failed to update environment for server %s: %w", server.Name, err)
	}

	// Run Kalman filter (predict + update)
	tunedResults, err := tuner.Run()
	if err != nil {
		return fmt.Errorf("tuner run failed: %w", err)
	}

	// update modelperf using tuned params
	if err := updateModelPerfDataInSystemData(systemData, server.Model, server.CurrentAlloc.Accelerator, tunedResults); err != nil {
		return fmt.Errorf("failed to update the params of server %s: %w", server.Name, err)
	}

	return nil
}

// Check if the server is in a transient state due to recent up scaling.
func (tm *TunerManager) skipIfInTransition(
	server *infernoConfig.ServerSpec,
) (bool, error) {
	tuner, exists := tm.tuners[server.Name]
	if !exists {
		return false, fmt.Errorf("no tuner found for server %s", server.Name)
	}
	env := tuner.GetEnvironment()
	if env == nil {
		return false, fmt.Errorf("no tuner environment initialized for server %s", server.Name)
	}
	if env.TimeStamp == nil {
		return false, fmt.Errorf("no timestamp set in tuner environment for server %s", server.Name)
	}
	pastNumReplicas := env.NumReplicas
	curNumReplicas := server.CurrentAlloc.NumReplicas
	timeout := env.TimeStamp.Add(constants.TransientDelaySeconds * time.Second)
	if pastNumReplicas > 0 && curNumReplicas > pastNumReplicas && time.Now().Before(timeout) {
		return true, nil
	}
	return false, nil
}

func (tm *TunerManager) getOrCreateTuner(
	systemData *infernoConfig.SystemData,
	server *infernoConfig.ServerSpec,
) (*tune.Tuner, error) {
	// Try to get existing tuner
	tuner, exists := tm.tuners[server.Name]
	if exists {
		return tuner, nil
	}

	/*
			if the tuner doesn't exist then create a new tuner

		    ------------------------- COMMENTS --------------------------------
			Onboarding a new tuner requires initializing the Kalman Filter.
			To facilitate the initialization, we require 'expected observations' on performance metrics and an initial state.
			For the latter, the current design choice is that the initial state is extracted from the model profile in the provided VariantAutoscaling CRD.
			This still requires basic offline benchmarking of the parameters on the user's part.
			On the other hand, it is obvious that the SLO expectation should dictate expected observations.
			------------------------------------------------------------------
	*/

	// extract initial parameters from system data
	initState, err := findInitStateInSystemData(systemData, server.Model, server.CurrentAlloc.Accelerator)
	if err != nil {
		return nil, fmt.Errorf("failed to find init state for model accelerator pair %s, %s: %w",
			server.Model, server.CurrentAlloc.Accelerator, err)
	}

	// Find SLO targets from system data
	slos, err := findSLOInSystemData(systemData, server.Model, server.Class)
	if err != nil {
		return nil, fmt.Errorf("failed to find SLO for model class pair %s, %s: %w",
			server.Model, server.Class, err)
	}

	// build tuner config from initial state and slo
	configData, err := BuildTunerConfig(initState, slos)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	env := ConvertAllocToEnvironment(server.CurrentAlloc)

	// validate environment before creating tuner
	if !env.Valid() {
		return nil, fmt.Errorf("invalid environment for server %s: environment validation failed", server.Name)
	}

	tuner, err = tune.NewTuner(configData, env)
	if err != nil {
		return nil, fmt.Errorf("failed to create tuner: %w", err)
	}

	// add the new tuner to existing list
	tm.tuners[server.Name] = tuner
	logger.Log.Info("Created new tuner",
		"server", server.Name,
		"initState", initState,
		"expected observations", slos)

	return tuner, nil
}

func (tm *TunerManager) RemoveTuners(systemData *infernoConfig.SystemData) {
	activeServers := make(map[string]bool)
	for _, server := range systemData.Spec.Servers.Spec {
		activeServers[server.Name] = true
	}

	// Remove tuners that are no longer in SystemData
	for serverName := range tm.tuners {
		if !activeServers[serverName] {
			delete(tm.tuners, serverName)
			logger.Log.Info("Removed tuner for deleted server", "server", serverName)
		}
	}
}
