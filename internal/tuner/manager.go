package controller

import (
	"fmt"
	"sync"

	llmdVariantAutoscalingV1alpha1 "github.com/llm-d-incubation/workload-variant-autoscaler/api/v1alpha1"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/utils"
	"github.com/llm-d-incubation/workload-variant-autoscaler/pkg/analyzer"
	infernoConfig "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
	tune "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/tuner"
	"gonum.org/v1/gonum/mat"
)

// TunerManager manages Kalman filter tuners for all VariantAutoscalings.
type TunerManager struct {
	tuners  map[string]*tune.Tuner
	mu      sync.RWMutex
	enabled bool // feature falg
}

// TunedResults holds the results of parameter tuning.
type TunedResults struct {
	ServiceParms *analyzer.ServiceParms
	Innovation   *mat.VecDense
	Covariance   *mat.Dense
}

const (
	/*
		Under nominal conditions, the NIS (Normalized Innovations Squared) of a Kalman Filter is expected to follow
		a Chi-Squared Distribution with degrees of freedom equal to the dimension of the measurement vector (n = 2 for [ttft, itl]).
		Here, we enforce that a tuner update is accepted for 95% confidence interval of NIS.
		The upper bound of the interval in our case is 7.378.
	*/
	MaxNIS float64 = 7.378
)

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

// TuneModelPerfParams tunes performance model parameters for all servers in SystemData.
func (tm *TunerManager) TuneModelPerfParams(systemData *infernoConfig.SystemData) error {
	// tune model tuner for each server
	for i := range systemData.Spec.Servers.Spec {
		server := &systemData.Spec.Servers.Spec[i]
		if err := tm.tuneServer(systemData, server); err != nil {
			logger.Log.Warn("Failed to tune server, keeping original params",
				"server", systemData.Spec.Servers.Spec[i].Name,
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
	if err := tuner.Run(); err != nil {
		return fmt.Errorf("tuner run failed: %w", err)
	}

	// Extract tuned parameters
	tunedResults, err := tm.extractTunedResults(tuner)
	if err != nil {
		return fmt.Errorf("failed to extract tuned params: %w", err)
	}

	// check validity of tunedResults
	if err := tm.validateTunedResults(tunedResults, tuner); err != nil {
		logger.Log.Warn("Tuned parameters failed validation, rejecting update",
			"server", server.Name,
			"error", err)
		return nil
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
	// Try to get existing
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
			Onboarding a new tuner requires initializing the Kalmn Filter.
			To facilitate the initialization, we require 'expected observations' on performance metrics and an initial state.
			For the latter, the current design choice is that the initial state is extracted from the model profile in the provided VA CRD.
			This still requires basic offline benchmarking of the parameters on the user's part.
			On the other hand, it is obvious that the slo expectation should dictate expected obsevrations.
			------------------------------------------------------------------
	*/

	// extract initial parameters from system data
	initState, err := findInitStateInSystemData(systemData, server.Model, server.CurrentAlloc.Accelerator)
	if err != nil {
		return nil, fmt.Errorf("failed to find init state for model accelerator pair %s, %s: %w",
			server.Model, server.CurrentAlloc.Accelerator,
			err)
	}

	// Find SLO targets from system data
	slos, err := findSLOInSystemData(systemData, server.Model, server.Class)
	if err != nil {
		return nil, fmt.Errorf("failed to find SLO for model class pair %s, %s: %w",
			server.Model, server.Class,
			err)
	}

	// build tuner config from initial state and slo
	configData, err := BuildTunerConfig(initState, slos)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	env := ConvertAllocToEnvironment(server.CurrentAlloc)

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

func (tm *TunerManager) extractTunedResults(tuner *tune.Tuner) (*TunedResults, error) {
	stateVec := tuner.X()
	if stateVec == nil {
		return nil, fmt.Errorf("tuner returned nil state vector")
	}
	innovation := tuner.Innovation()
	covariance := tuner.P()

	return &TunedResults{
		ServiceParms: &analyzer.ServiceParms{
			Decode: &analyzer.DecodeParms{
				Alpha: float32(stateVec.AtVec(0)),
				Beta:  float32(stateVec.AtVec(1)),
			},
			Prefill: &analyzer.PrefillParms{
				Gamma: float32(stateVec.AtVec(2)),
				Delta: float32(stateVec.AtVec(3)),
			},
		},
		Innovation: innovation, // or maybe return the copy
		Covariance: covariance,
	}, nil
}

func (tm *TunerManager) RemoveTuners(items []llmdVariantAutoscalingV1alpha1.VariantAutoscaling) {
	for _, va := range items {
		if !va.DeletionTimestamp.IsZero() {
			serverName := utils.FullName(va.Name, va.Namespace)
			tm.removeTuner(serverName)
		}
	}

}

// removeTuner removes a tuner by server name
func (tm *TunerManager) removeTuner(serverName string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	delete(tm.tuners, serverName)
	logger.Log.Info("Removed tuner", "server", serverName)
}

func (tm *TunerManager) validateTunedResults(tunedResults *TunedResults, tuner *tune.Tuner) error {
	parms := tunedResults.ServiceParms

	// 1. check parms are positive
	if parms.Decode.Alpha <= 0 || parms.Decode.Beta <= 0 {
		return fmt.Errorf("decode parameters must be positive: alpha=%f, beta=%f", parms.Decode.Alpha, parms.Decode.Beta)
	}
	if parms.Prefill.Gamma <= 0 || parms.Prefill.Delta <= 0 {
		return fmt.Errorf("prefill parameters must be positive: gamma=%f, delta=%f", parms.Prefill.Gamma, parms.Prefill.Delta)
	}

	// 2. innovation check using Normalized Innovation Squared (NIS)
	innovation := tuner.Innovation() // y vector
	innovationCov := tuner.S()       // S matrix

	// Calculate NIS = y^T * S^-1 * y
	S_inv := mat.NewDense(innovationCov.RawMatrix().Rows, innovationCov.RawMatrix().Cols, nil)
	if err := S_inv.Inverse(innovationCov); err != nil {
		return fmt.Errorf("singular innovation covariance matrix S encountered: %w", err)
	}

	// tmp = S^-1 * y
	tmp := mat.NewVecDense(S_inv.RawMatrix().Rows, nil)
	tmp.MulVec(S_inv, innovation)

	// NIS = y^T * tmp
	NIS := mat.Dot(innovation, tmp)

	if NIS >= MaxNIS {
		return fmt.Errorf("normalized innovation squared (NIS=%.2f) exceeds threshold (%.2f), rejecting update as outlier",
			NIS, MaxNIS)
	}

	// 3. estimate covariance check?
	// TODO

	return nil
}
