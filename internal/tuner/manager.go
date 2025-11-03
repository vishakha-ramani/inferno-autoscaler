package controller

import (
	"fmt"
	"sync"

	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	infernoConfig "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
	tune "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/tuner"
)

// TunerManager manages Kalman filter tuners for all VariantAutoscalings.
// Assumption: Server environments are assumed to be prefill and decode.
type TunerManager struct {
	// Map of server (variant) name to its tuner
	tuners map[string]*tune.Tuner
	// Mutex for concurrent access to tuners map
	mu sync.RWMutex
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
	// Check if tuning is disabled
	if !tm.enabled {
		logger.Log.Debug("Tuning is disabled, skipping parameter tuning")
		return nil
	}

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
	tuner.UpdateEnvironment(env)

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

func (tm *TunerManager) getOrCreateTuner(
	systemData *infernoConfig.SystemData,
	server *infernoConfig.ServerSpec,
) (*tune.Tuner, error) {
	// Try to get existing tuner
	tm.mu.RLock()
	tuner, exists := tm.tuners[server.Name]
	tm.mu.RUnlock()

	if exists {
		return tuner, nil
	}

	// Create new (write lock)
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check after acquiring write lock
	if tuner, exists := tm.tuners[server.Name]; exists {
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
	tm.mu.Lock()
	defer tm.mu.Unlock()

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
