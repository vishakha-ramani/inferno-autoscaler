package tuner

import (
	"math"
	"strings"
	"testing"

	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/constants"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	"github.com/llm-d-incubation/workload-variant-autoscaler/pkg/analyzer"
	"go.uber.org/zap"
	"gonum.org/v1/gonum/mat"
)

// Helper to initialize logger for tests
func init() {
	logger.Log = zap.NewNop().Sugar()
}

// Helper function to create a valid test environment
func createTestEnvironment() *Environment {
	return &Environment{
		Lambda:        60.0, // 60 requests per minute = 1 req/sec
		AvgInputToks:  100,  // 100 input tokens
		AvgOutputToks: 200,  // 200 output tokens
		MaxBatchSize:  8,    // max batch size of 8
		AvgTTFT:       190,  // 190 ms TTFT - actual prediction from queue analyzer
		AvgITL:        15,   // 15 ms ITL - actual prediction from queue analyzer
	}
}

// Helper function to create valid test configuration
func createTestConfig() *TunerConfigData {
	return &TunerConfigData{
		FilterData: FilterData{
			GammaFactor: constants.DefaultGammaFactor,
			ErrorLevel:  constants.DefaultErrorLevel,
			TPercentile: constants.DefaultTPercentile,
		},
		ModelData: TunerModelData{
			InitState:            []float64{5.0, 2.5, 10.0, 0.15}, // alpha, beta, gamma, delta - values that produce the observations above
			PercentChange:        []float64{0.1, 0.1, 0.1, 0.1},   // 10% uncertainty
			BoundedState:         true,
			MinState:             []float64{1.0, 0.5, 2.0, 0.01},
			MaxState:             []float64{20.0, 10.0, 50.0, 1.0},
			ExpectedObservations: []float64{190, 15.0}, // Expected TTFT and ITL - matches actual analyzer output
		},
	}
}

func TestNewTuner(t *testing.T) {
	tests := []struct {
		name       string
		configData *TunerConfigData
		env        *Environment
		wantErr    bool
	}{
		{
			name:       "valid configuration",
			configData: createTestConfig(),
			env:        createTestEnvironment(),
			wantErr:    false,
		},
		{
			name:       "nil config data",
			configData: nil,
			env:        createTestEnvironment(),
			wantErr:    true,
		},
		{
			name:       "nil environment",
			configData: createTestConfig(),
			env:        nil,
			wantErr:    true,
		},
		{
			name: "empty init state",
			configData: &TunerConfigData{
				FilterData: FilterData{
					GammaFactor: 1.0,
					ErrorLevel:  0.05,
					TPercentile: 1.96,
				},
				ModelData: TunerModelData{
					InitState:            []float64{},
					PercentChange:        []float64{},
					BoundedState:         false,
					ExpectedObservations: []float64{150.0, 25.0},
				},
			},
			env:     createTestEnvironment(),
			wantErr: true,
		},
		{
			name: "mismatched state dimensions",
			configData: &TunerConfigData{
				FilterData: FilterData{
					GammaFactor: 1.0,
					ErrorLevel:  0.05,
					TPercentile: 1.96,
				},
				ModelData: TunerModelData{
					InitState:            []float64{5.0, 2.0},
					PercentChange:        []float64{0.05, 0.05, 0.05}, // Mismatch
					BoundedState:         false,
					ExpectedObservations: []float64{150.0, 25.0},
				},
			},
			env:     createTestEnvironment(),
			wantErr: true,
		},
		{
			name: "bounded state with missing bounds",
			configData: &TunerConfigData{
				FilterData: FilterData{
					GammaFactor: 1.0,
					ErrorLevel:  0.05,
					TPercentile: 1.96,
				},
				ModelData: TunerModelData{
					InitState:            []float64{5.0, 2.0, 10.0, 1.5},
					PercentChange:        []float64{0.05, 0.05, 0.05, 0.05},
					BoundedState:         true,
					MinState:             []float64{0.5, 0.2}, // Incomplete
					MaxState:             []float64{50.0, 20.0, 100.0, 15.0},
					ExpectedObservations: []float64{150.0, 25.0},
				},
			},
			env:     createTestEnvironment(),
			wantErr: true,
		},
		{
			name: "zero expected observations",
			configData: &TunerConfigData{
				FilterData: FilterData{
					GammaFactor: 1.0,
					ErrorLevel:  0.05,
					TPercentile: 1.96,
				},
				ModelData: TunerModelData{
					InitState:            []float64{5.0, 2.0, 1.5, 10.0},
					PercentChange:        []float64{0.05, 0.05, 0.05, 0.05},
					BoundedState:         false,
					ExpectedObservations: []float64{}, // Empty
				},
			},
			env:     createTestEnvironment(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tuner, err := NewTuner(tt.configData, tt.env)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTuner() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tuner == nil {
				t.Error("NewTuner() returned nil tuner without error")
			}
		})
	}
}

func TestTuner_Run(t *testing.T) {
	tests := []struct {
		name       string
		configData *TunerConfigData
		env        *Environment
		wantErr    bool
		wantNewErr bool // expect error in NewTuner
		checkFunc  func(*testing.T, *TunedResults)
	}{
		{
			name:       "successful tuning with valid data",
			configData: createTestConfig(),
			env:        createTestEnvironment(),
			wantErr:    false,
			wantNewErr: false,
			checkFunc: func(t *testing.T, results *TunedResults) {
				if results == nil {
					t.Error("TunedResults is nil")
					return
				}
				if results.ServiceParms == nil {
					t.Error("ServiceParms is nil")
					return
				}
				// Check that parameters are positive
				if results.ServiceParms.Decode.Alpha <= 0 {
					t.Error("Alpha must be positive")
				}
				if results.ServiceParms.Decode.Beta <= 0 {
					t.Error("Beta must be positive")
				}
				if results.ServiceParms.Prefill.Gamma <= 0 {
					t.Error("Gamma must be positive")
				}
				if results.ServiceParms.Prefill.Delta <= 0 {
					t.Error("Delta must be positive")
				}
				if results.Innovation == nil {
					t.Error("Innovation vector is nil")
				}
				if results.Covariance == nil {
					t.Error("Covariance matrix is nil")
				}
			},
		},
		{
			name:       "invalid environment - zero lambda",
			configData: createTestConfig(),
			env: &Environment{
				Lambda:        0.0, // Invalid lambda
				AvgInputToks:  100,
				AvgOutputToks: 200,
				MaxBatchSize:  8,
				AvgTTFT:       150.0,
				AvgITL:        25.0,
			},
			wantNewErr: true, // NewTuner should fail
		},
		{
			name:       "invalid environment - zero batch size",
			configData: createTestConfig(),
			env: &Environment{
				Lambda:        60.0,
				AvgInputToks:  100,
				AvgOutputToks: 200,
				MaxBatchSize:  0, // Invalid max batch size
				AvgTTFT:       150.0,
				AvgITL:        25.0,
			},
			wantNewErr: true, // NewTuner should fail
		},
		{
			name:       "extreme observations causing high NIS",
			configData: createTestConfig(),
			env: &Environment{
				Lambda:        60.0,
				AvgInputToks:  100,
				AvgOutputToks: 200,
				MaxBatchSize:  8,
				AvgTTFT:       10000.0, // Extremely high, should trigger NIS rejection
				AvgITL:        5000.0,  // Extremely high
			},
			wantErr:    true, // Run() should fail NIS validation
			wantNewErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tuner, err := NewTuner(tt.configData, tt.env)
			if tt.wantNewErr {
				if err == nil {
					t.Errorf("NewTuner() expected error but got none")
				}
				return // Skip Run() test if NewTuner should fail
			}
			if err != nil {
				t.Fatalf("Failed to create tuner: %v", err)
			}

			results, err := tuner.Run()
			if (err != nil) != tt.wantErr {
				t.Errorf("Tuner.Run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, results)
			}
		})
	}
}

func TestTuner_UpdateEnvironment(t *testing.T) {
	configData := createTestConfig()
	initialEnv := createTestEnvironment()

	tuner, err := NewTuner(configData, initialEnv)
	if err != nil {
		t.Fatalf("Failed to create tuner: %v", err)
	}

	// Update environment with modified observations but same structure
	// Use values close to predictions to simulate small measurement variations
	newEnv := &Environment{
		Lambda:        60.0, // Same rate
		AvgInputToks:  100,  // Same tokens
		AvgOutputToks: 200,
		MaxBatchSize:  8,     // Same batch size
		AvgTTFT:       190.0, // Small variation from 186.7
		AvgITL:        15.5,  // Small variation from 14.9
	}

	if err := tuner.UpdateEnvironment(newEnv); err != nil {
		t.Fatalf("UpdateEnvironment() failed: %v", err)
	}

	// Run tuning with new environment
	results, err := tuner.Run()
	if err != nil {
		t.Fatalf("Tuner.Run() failed after environment update: %v", err)
	}

	if results == nil {
		t.Error("TunedResults is nil after environment update")
	}
}

func TestTuner_GetParms(t *testing.T) {
	configData := createTestConfig()
	env := createTestEnvironment()

	tuner, err := NewTuner(configData, env)
	if err != nil {
		t.Fatalf("Failed to create tuner: %v", err)
	}

	parms := tuner.GetParms()
	if parms == nil {
		t.Fatal("GetParms() returned nil")
	}

	// Check dimensions
	if parms.Len() != 4 {
		t.Errorf("Expected 4 parameters, got %d", parms.Len())
	}

	// Check that initial parameters match config
	expectedParms := configData.ModelData.InitState
	for i := 0; i < parms.Len(); i++ {
		if math.Abs(parms.AtVec(i)-expectedParms[i]) > 1e-6 {
			t.Errorf("Parameter %d: expected %f, got %f", i, expectedParms[i], parms.AtVec(i))
		}
	}
}

func TestTuner_StateAccessors(t *testing.T) {
	configData := createTestConfig()
	env := createTestEnvironment()

	tuner, err := NewTuner(configData, env)
	if err != nil {
		t.Fatalf("Failed to create tuner: %v", err)
	}

	// Test X() - state vector
	x := tuner.X()
	if x == nil {
		t.Error("X() returned nil")
	}
	if x.Len() != 4 {
		t.Errorf("Expected state dimension 4, got %d", x.Len())
	}

	// Test P() - covariance matrix
	p := tuner.P()
	if p == nil {
		t.Error("P() returned nil")
	}
	rows, cols := p.Dims()
	if rows != 4 || cols != 4 {
		t.Errorf("Expected P dimension 4x4, got %dx%d", rows, cols)
	}

	// Run to populate innovation
	_, err = tuner.Run()
	if err != nil {
		t.Logf("Run() failed (this may be expected): %v", err)
	}

	// Test Y() - innovation vector
	y := tuner.Y()
	if y == nil {
		t.Error("Y() returned nil")
	}

	// Test S() - innovation covariance
	s := tuner.S()
	if s == nil {
		t.Error("S() returned nil")
	}
}

func TestTuner_ValidatePositiveParameters(t *testing.T) {
	// Test that tuned parameters remain positive after successful tuning
	configData := createTestConfig()
	env := createTestEnvironment()

	tuner, err := NewTuner(configData, env)
	if err != nil {
		t.Fatalf("Failed to create tuner: %v", err)
	}

	// Run tuning
	results, err := tuner.Run()
	if err != nil {
		t.Fatalf("Tuner.Run() failed: %v", err)
	}

	// Validate all parameters are positive
	if results.ServiceParms.Decode.Alpha <= 0 {
		t.Errorf("Alpha must be positive, got %f", results.ServiceParms.Decode.Alpha)
	}
	if results.ServiceParms.Decode.Beta <= 0 {
		t.Errorf("Beta must be positive, got %f", results.ServiceParms.Decode.Beta)
	}
	if results.ServiceParms.Prefill.Delta <= 0 {
		t.Errorf("Delta must be positive, got %f", results.ServiceParms.Prefill.Delta)
	}
	if results.ServiceParms.Prefill.Gamma <= 0 {
		t.Errorf("Gamma must be positive, got %f", results.ServiceParms.Prefill.Gamma)
	}

	t.Logf("All parameters positive: Alpha=%.3f, Beta=%.3f, Delta=%.4f, Gamma=%.3f",
		results.ServiceParms.Decode.Alpha,
		results.ServiceParms.Decode.Beta,
		results.ServiceParms.Prefill.Delta,
		results.ServiceParms.Prefill.Gamma)
}

func TestTuner_MultipleIterations(t *testing.T) {
	configData := createTestConfig()
	env := createTestEnvironment()

	tuner, err := NewTuner(configData, env)
	if err != nil {
		t.Fatalf("Failed to create tuner: %v", err)
	}

	// Run multiple tuning iterations
	numIterations := 5
	var previousAlpha float32 = -1

	for i := range numIterations {
		results, err := tuner.Run()
		if err != nil {
			t.Logf("Iteration %d failed: %v", i, err)
			continue
		}

		if results == nil || results.ServiceParms == nil {
			t.Errorf("Iteration %d: Got nil results", i)
			continue
		}

		alpha := results.ServiceParms.Decode.Alpha
		t.Logf("Iteration %d: Alpha = %f", i, alpha)

		// Check that parameter is still positive
		if alpha <= 0 {
			t.Errorf("Iteration %d: Alpha became non-positive: %f", i, alpha)
		}

		if previousAlpha > 0 {
			// Parameters should converge, so change should decrease over time
			t.Logf("Alpha change: %f", math.Abs(float64(alpha-previousAlpha)))
		}

		previousAlpha = alpha
	}
}

func TestTuner_String(t *testing.T) {
	configData := createTestConfig()
	env := createTestEnvironment()

	tuner, err := NewTuner(configData, env)
	if err != nil {
		t.Fatalf("Failed to create tuner: %v", err)
	}

	str := tuner.String()
	if str == "" {
		t.Error("String() returned empty string")
	}

	// Check that string contains expected components
	expectedSubstrings := []string{"Tuner:", "Configurator:", "Environment:"}
	for _, substr := range expectedSubstrings {
		if !strings.Contains(str, substr) {
			t.Errorf("String() output missing expected substring: %s", substr)
		}
	}
}

func TestTunedResults_ParameterExtraction(t *testing.T) {
	configData := createTestConfig()
	env := createTestEnvironment()

	tuner, err := NewTuner(configData, env)
	if err != nil {
		t.Fatalf("Failed to create tuner: %v", err)
	}

	results, err := tuner.Run()
	if err != nil {
		t.Skipf("Tuner.Run() failed: %v", err)
	}

	if results == nil {
		t.Fatal("TunedResults is nil")
	}

	// Verify ServiceParms structure
	if results.ServiceParms == nil {
		t.Fatal("ServiceParms is nil")
	}

	// Verify Decode parameters
	decode := results.ServiceParms.Decode
	if decode == nil {
		t.Fatal("Decode parameters are nil")
	}
	if decode.Alpha <= 0 || decode.Beta <= 0 {
		t.Errorf("Invalid Decode parameters: Alpha=%f, Beta=%f", decode.Alpha, decode.Beta)
	}

	// Verify Prefill parameters
	prefill := results.ServiceParms.Prefill
	if prefill == nil {
		t.Fatal("Prefill parameters are nil")
	}
	if prefill.Gamma <= 0 || prefill.Delta <= 0 {
		t.Errorf("Invalid Prefill parameters: Gamma=%f, Delta=%f", prefill.Gamma, prefill.Delta)
	}

	// Verify Innovation vector
	if results.Innovation == nil {
		t.Error("Innovation vector is nil")
	} else if results.Innovation.Len() != 2 {
		t.Errorf("Innovation vector should have length 2, got %d", results.Innovation.Len())
	}

	// Verify Covariance matrix
	if results.Covariance == nil {
		t.Error("Covariance matrix is nil")
	} else {
		rows, cols := results.Covariance.Dims()
		if rows != 4 || cols != 4 {
			t.Errorf("Covariance matrix should be 4x4, got %dx%d", rows, cols)
		}
	}
}

func TestTuner_BoundedStateEnforcement(t *testing.T) {
	configData := &TunerConfigData{
		FilterData: FilterData{
			GammaFactor: 1.0,
			ErrorLevel:  0.05,
			TPercentile: 1.96,
		},
		ModelData: TunerModelData{
			InitState:            []float64{5.0, 2.5, 10.0, 0.15}, // Use realistic params
			PercentChange:        []float64{0.1, 0.1, 0.1, 0.1},
			BoundedState:         true,
			MinState:             []float64{4.0, 2.0, 8.0, 0.1}, // Tight bounds: alpha, beta, gamma, delta
			MaxState:             []float64{6.0, 3.0, 12.0, 0.2},
			ExpectedObservations: []float64{186.7, 14.9}, // Use realistic values
		},
	}

	env := createTestEnvironment()

	tuner, err := NewTuner(configData, env)
	if err != nil {
		t.Fatalf("Failed to create tuner: %v", err)
	}

	// Run multiple iterations - should succeed with realistic params
	successCount := 0
	for i := range 10 {
		results, err := tuner.Run()
		if err != nil {
			t.Logf("Iteration %d: %v", i, err)
			continue
		}

		successCount++

		if results == nil || results.ServiceParms == nil {
			continue
		}

		// Check that parameters are within bounds
		alpha := results.ServiceParms.Decode.Alpha
		beta := results.ServiceParms.Decode.Beta
		delta := results.ServiceParms.Prefill.Delta
		gamma := results.ServiceParms.Prefill.Gamma

		if alpha < float32(configData.ModelData.MinState[StateIndexAlpha]) || alpha > float32(configData.ModelData.MaxState[StateIndexAlpha]) {
			t.Errorf("Alpha %f out of bounds [%f, %f]", alpha, configData.ModelData.MinState[StateIndexAlpha], configData.ModelData.MaxState[StateIndexAlpha])
		}
		if beta < float32(configData.ModelData.MinState[StateIndexBeta]) || beta > float32(configData.ModelData.MaxState[StateIndexBeta]) {
			t.Errorf("Beta %f out of bounds [%f, %f]", beta, configData.ModelData.MinState[StateIndexBeta], configData.ModelData.MaxState[StateIndexBeta])
		}
		if gamma < float32(configData.ModelData.MinState[StateIndexGamma]) || gamma > float32(configData.ModelData.MaxState[StateIndexGamma]) {
			t.Errorf("Gamma %f out of bounds [%f, %f]", gamma, configData.ModelData.MinState[StateIndexGamma], configData.ModelData.MaxState[StateIndexGamma])
		}
		if delta < float32(configData.ModelData.MinState[StateIndexDelta]) || delta > float32(configData.ModelData.MaxState[StateIndexDelta]) {
			t.Errorf("Delta %f out of bounds [%f, %f]", delta, configData.ModelData.MinState[StateIndexDelta], configData.ModelData.MaxState[StateIndexDelta])
		}
	}

	// Verify we had some successful runs
	if successCount == 0 {
		t.Error("No successful tuning iterations - all were rejected by NIS validation")
	} else {
		t.Logf("Successfully completed %d out of 10 iterations", successCount)
	}
}

func TestTuner_ConvergenceWithMultipleInitialGuesses(t *testing.T) {
	observedTTFT := 186.7 // ms
	observedITL := 14.9   // ms

	avgInputToks := 512
	avgOutputToks := 128
	lambda := 60.0 // req/min
	batchSize := 8

	t.Logf("\n TEST CONFIGURATION:")
	t.Logf("   Observed: TTFT=%.1f ms, ITL=%.1f ms", observedTTFT, observedITL)
	t.Logf("   Workload: λ=%.0f req/min, input=%d toks, output=%d toks, batch=%d",
		lambda, avgInputToks, avgOutputToks, batchSize)

	testCases := []struct {
		name        string
		initAlpha   float64
		initBeta    float64
		initGamma   float64
		initDelta   float64
		description string
	}{
		{
			name:        "close_guess",
			initAlpha:   8.5,
			initBeta:    2.1,
			initGamma:   5.0,
			initDelta:   0.11,
			description: "Close to optimal (within 5-10%)",
		},
		{
			name:        "moderate_off",
			initAlpha:   7.0,
			initBeta:    2.5,
			initGamma:   6.0,
			initDelta:   0.09,
			description: "Moderately off (~15-20%)",
		},
		{
			name:        "significant_error",
			initAlpha:   5.0,
			initBeta:    3.0,
			initGamma:   8.0,
			initDelta:   0.15,
			description: "Significant initial error (~40-60%)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("\n %s: α=%.1f β=%.1f γ=%.1f δ=%.3f",
				tc.description, tc.initAlpha, tc.initBeta, tc.initGamma, tc.initDelta)

			// Calculate initial prediction error
			initialPred := predictWithQueueAnalyzer(tc.initAlpha, tc.initBeta, tc.initGamma, tc.initDelta,
				batchSize, avgInputToks, avgOutputToks, lambda)

			initialError := math.Abs(float64(initialPred.ttft-float32(observedTTFT))) +
				math.Abs(float64(initialPred.itl-float32(observedITL)))

			t.Logf("   Initial prediction: TTFT=%.1f ITL=%.1f (error: %.1f ms)",
				initialPred.ttft, initialPred.itl, initialError)

			// Configure tuner
			configData := createTestConfig()
			configData.ModelData.InitState = []float64{tc.initAlpha, tc.initBeta, tc.initGamma, tc.initDelta}
			configData.ModelData.ExpectedObservations = []float64{observedTTFT, observedITL}

			env := &Environment{
				Lambda:        float32(lambda),
				AvgInputToks:  avgInputToks,
				AvgOutputToks: avgOutputToks,
				MaxBatchSize:  batchSize,
				AvgTTFT:       float32(observedTTFT),
				AvgITL:        float32(observedITL),
			}

			tuner, err := NewTuner(configData, env)
			if err != nil {
				t.Fatalf("Failed to create tuner: %v", err)
			}

			// Run tuning iterations
			numIterations := 20
			successCount := 0
			rejectedCount := 0

			type snapshot struct {
				iter                      int
				alpha, beta, gamma, delta float64
				ttft, itl, error          float32
			}
			var milestones []snapshot

			// Record initial
			params := tuner.GetParms()
			milestones = append(milestones, snapshot{
				iter:  0,
				alpha: params.AtVec(StateIndexAlpha),
				beta:  params.AtVec(StateIndexBeta),
				gamma: params.AtVec(StateIndexGamma),
				delta: params.AtVec(StateIndexDelta),
				ttft:  initialPred.ttft,
				itl:   initialPred.itl,
				error: float32(initialError),
			})

			// Run iterations
			for i := 1; i <= numIterations; i++ {
				_, err := tuner.Run()
				if err != nil {
					rejectedCount++
					if i%10 == 0 {
						t.Logf("   Iter %d: Update rejected (%v)", i, err)
					}
					continue
				}

				successCount++
				params := tuner.GetParms()
				alpha := params.AtVec(StateIndexAlpha)
				beta := params.AtVec(StateIndexBeta)
				gamma := params.AtVec(StateIndexGamma)
				delta := params.AtVec(StateIndexDelta)

				pred := predictWithQueueAnalyzer(alpha, beta, gamma, delta,
					batchSize, avgInputToks, avgOutputToks, lambda)

				iterError := math.Abs(float64(pred.ttft-float32(observedTTFT))) +
					math.Abs(float64(pred.itl-float32(observedITL)))

				// Record milestones
				if i == 1 || i == 5 || i == 10 || i == numIterations || iterError < 1.0 {
					milestones = append(milestones, snapshot{
						iter:  i,
						alpha: alpha,
						beta:  beta,
						gamma: gamma,
						delta: delta,
						ttft:  pred.ttft,
						itl:   pred.itl,
						error: float32(iterError),
					})

					if iterError < 1.0 && len(milestones) > 1 && milestones[len(milestones)-2].error >= 1.0 {
						t.Logf("   ✓ Iter %d: Converged! error=%.1f ms", i, iterError)
					}
				}
			}

			// Print milestone trajectory
			t.Logf("\n   Convergence milestones:")
			t.Logf("   %-4s  %7s %7s %7s %7s  %8s %7s  %7s",
				"Iter", "Alpha", "Beta", "Gamma", "Delta", "TTFT", "ITL", "Error")
			for _, m := range milestones {
				t.Logf("   %-4d  %7.2f %7.2f %7.2f %7.4f  %8.1f %7.1f  %7.1f",
					m.iter, m.alpha, m.beta, m.gamma, m.delta, m.ttft, m.itl, m.error)
			}

			// Final evaluation
			final := milestones[len(milestones)-1]
			improvement := float64(initialError) - float64(final.error)
			improvementPct := 100 * improvement / initialError

			t.Logf("\n   📈 Results:")
			t.Logf("      Success: %d/%d iterations (%.0f%%)", successCount, numIterations, 100*float64(successCount)/float64(numIterations))
			t.Logf("      Error: %.1f → %.1f ms (%.1f%% improvement)", initialError, final.error, improvementPct)

			// Validate convergence
			if successCount > 0 {
				if final.error >= float32(initialError) {
					t.Errorf("Failed to improve: error %.1f → %.1f ms", initialError, final.error)
				}
				if final.error > 10.0 && successCount > 10 {
					t.Logf("      Warning: Final error still high (%.1f ms)", final.error)
				}
				if final.error < 1.0 {
					t.Logf("      Excellent convergence (error < 1ms)")
				}
			} else {
				t.Errorf("      All updates rejected - initial guess may be too far off")
			}

			// Verify bounds
			validateParameterBounds(t, final.alpha, final.beta, final.gamma, final.delta)
		})
	}
}

// Helper type for prediction results
type predictionResult struct {
	ttft float32
	itl  float32
}

// Helper to predict using queue analyzer
func predictWithQueueAnalyzer(alpha, beta, gamma, delta float64,
	batchSize, avgInputToks, avgOutputToks int, lambda float64) predictionResult {

	qConfig := &analyzer.Configuration{
		MaxBatchSize: batchSize,
		MaxQueueSize: batchSize * 5,
		ServiceParms: &analyzer.ServiceParms{
			Prefill: &analyzer.PrefillParms{
				Gamma: float32(gamma),
				Delta: float32(delta),
			},
			Decode: &analyzer.DecodeParms{
				Alpha: float32(alpha),
				Beta:  float32(beta),
			},
		},
	}

	requestData := &analyzer.RequestSize{
		AvgInputTokens:  avgInputToks,
		AvgOutputTokens: avgOutputToks,
	}

	qa, err := analyzer.NewQueueAnalyzer(qConfig, requestData)
	if err != nil {
		return predictionResult{}
	}

	metrics, err := qa.Analyze(float32(lambda / 60.0))
	if err != nil {
		return predictionResult{}
	}

	return predictionResult{
		ttft: metrics.AvgWaitTime + metrics.AvgPrefillTime,
		itl:  metrics.AvgTokenTime,
	}
}

// Helper to validate parameter bounds
func validateParameterBounds(t *testing.T, alpha, beta, gamma, delta float64) {
	configLimits := createTestConfig()

	if alpha < configLimits.ModelData.MinState[StateIndexAlpha] ||
		alpha > configLimits.ModelData.MaxState[StateIndexAlpha] {
		t.Errorf("Alpha %.3f out of bounds [%.1f, %.1f]",
			alpha, configLimits.ModelData.MinState[StateIndexAlpha], configLimits.ModelData.MaxState[StateIndexAlpha])
	}
	if beta < configLimits.ModelData.MinState[StateIndexBeta] ||
		beta > configLimits.ModelData.MaxState[StateIndexBeta] {
		t.Errorf("Beta %.3f out of bounds [%.1f, %.1f]",
			beta, configLimits.ModelData.MinState[StateIndexBeta], configLimits.ModelData.MaxState[StateIndexBeta])
	}
	if gamma < configLimits.ModelData.MinState[StateIndexGamma] ||
		gamma > configLimits.ModelData.MaxState[StateIndexGamma] {
		t.Errorf("Gamma %.3f out of bounds [%.1f, %.1f]",
			gamma, configLimits.ModelData.MinState[StateIndexGamma], configLimits.ModelData.MaxState[StateIndexGamma])
	}
	if delta < configLimits.ModelData.MinState[StateIndexDelta] ||
		delta > configLimits.ModelData.MaxState[StateIndexDelta] {
		t.Errorf("Delta %.4f out of bounds [%.2f, %.2f]",
			delta, configLimits.ModelData.MinState[StateIndexDelta], configLimits.ModelData.MaxState[StateIndexDelta])
	}

	if alpha <= 0 || beta <= 0 || gamma <= 0 || delta <= 0 {
		t.Errorf("Parameters must be positive: α=%.3f β=%.3f γ=%.3f δ=%.4f",
			alpha, beta, gamma, delta)
	}
}

// TestTuner_ErrorPaths tests error handling paths for improved coverage
func TestTuner_ErrorPaths(t *testing.T) {
	t.Run("nil_environment_in_update", func(t *testing.T) {
		configData := createTestConfig()
		env := createTestEnvironment()
		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("Failed to create tuner: %v", err)
		}

		err = tuner.UpdateEnvironment(nil)
		if err == nil {
			t.Error("Expected error when updating with nil environment")
		}
		if !strings.Contains(err.Error(), "environment cannot be nil") {
			t.Errorf("Expected 'environment cannot be nil' error, got: %v", err)
		}
	})

	t.Run("invalid_environment_in_update", func(t *testing.T) {
		configData := createTestConfig()
		env := createTestEnvironment()
		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("Failed to create tuner: %v", err)
		}

		// Create invalid environment
		invalidEnv := &Environment{
			Lambda:        0, // Invalid: zero lambda
			AvgInputToks:  100,
			AvgOutputToks: 200,
			MaxBatchSize:  8,
			AvgTTFT:       190,
			AvgITL:        15,
		}

		err = tuner.UpdateEnvironment(invalidEnv)
		if err == nil {
			t.Error("Expected error when updating with invalid environment")
		}
		if !strings.Contains(err.Error(), "invalid environment") {
			t.Errorf("Expected 'invalid environment' error, got: %v", err)
		}
	})

	t.Run("invalid_config_percent_change_mismatch", func(t *testing.T) {
		configData := createTestConfig()
		// Create mismatch between InitState and PercentChange
		configData.ModelData.InitState = []float64{5.0, 2.5, 10.0, 0.15}
		configData.ModelData.PercentChange = []float64{0.1, 0.1} // Wrong size!

		env := createTestEnvironment()
		_, err := NewTuner(configData, env)
		if err == nil {
			t.Error("Expected error when creating tuner with mismatched PercentChange")
		}
	})
}

func TestTuner_RunWithStasherFailures(t *testing.T) {
	t.Run("stash_failure", func(t *testing.T) {
		configData := createTestConfig()
		env := createTestEnvironment()

		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("NewTuner failed: %v", err)
		}

		// Corrupt filter state to cause stash failure
		tuner.filter.X = nil
		tuner.filter.P = nil

		_, err = tuner.Run()
		if err == nil {
			t.Error("Expected error when stashing fails")
		}
		if err != nil && !strings.Contains(err.Error(), "failed to stash") {
			t.Errorf("Wrong error message: %v", err)
		}
	})

	t.Run("unstash_after_validation_failure", func(t *testing.T) {
		configData := createTestConfig()
		env := &Environment{
			Lambda:        60.0,
			AvgInputToks:  100,
			AvgOutputToks: 200,
			MaxBatchSize:  8,
			AvgTTFT:       10000.0, // Extremely high to trigger NIS rejection
			AvgITL:        5000.0,
		}

		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("NewTuner failed: %v", err)
		}

		// Run should fail validation and attempt to unstash
		_, err = tuner.Run()
		if err == nil {
			t.Error("Expected error for extreme observations")
		}
		// The error should be about validation, not unstashing
		if err != nil && !strings.Contains(err.Error(), "validation failed") {
			t.Errorf("Expected validation error, got: %v", err)
		}
	})
}

func TestTuner_MakeObservationFuncErrorPaths(t *testing.T) {
	t.Run("invalid_state_parameters", func(t *testing.T) {
		configData := createTestConfig()
		env := createTestEnvironment()

		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("NewTuner failed: %v", err)
		}

		obsFunc := tuner.makeObservationFunc()

		// Test with invalid parameters (negative values)
		invalidState := mat.NewVecDense(4, []float64{-1.0, -2.0, -3.0, -0.1})
		result := obsFunc(invalidState)

		// With invalid parameters, queue analyzer should fail and return nil
		if result != nil {
			t.Error("Expected nil result for invalid parameters")
		}
	})

	t.Run("zero_state_parameters", func(t *testing.T) {
		configData := createTestConfig()
		env := createTestEnvironment()

		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("NewTuner failed: %v", err)
		}

		obsFunc := tuner.makeObservationFunc()

		// Test with zero parameters
		zeroState := mat.NewVecDense(4, []float64{0.0, 0.0, 0.0, 0.0})
		result := obsFunc(zeroState)

		// Zero parameters may or may not cause analyzer to fail depending on implementation
		// Just log the result
		if result == nil {
			t.Fatalf("Observation function returned nil for parameters all zero")
		} else {
			t.Logf("Observation function handled parameters: TTFT=%.2f, ITL=%.2f",
				result.AtVec(0), result.AtVec(1))
		}
	})
}

func TestTuner_ValidateTunedResultsNegativeParameters(t *testing.T) {
	t.Run("negative_alpha", func(t *testing.T) {
		configData := createTestConfig()
		env := createTestEnvironment()

		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("NewTuner failed: %v", err)
		}

		results := &TunedResults{
			ServiceParms: &analyzer.ServiceParms{
				Decode: &analyzer.DecodeParms{
					Alpha: -1.0, // Negative
					Beta:  2.5,
				},
				Prefill: &analyzer.PrefillParms{
					Gamma: 10.0,
					Delta: 0.15,
				},
			},
		}

		err = tuner.validateTunedResults(results)
		if err == nil {
			t.Error("Expected error for negative alpha")
		}
		if err != nil && !strings.Contains(err.Error(), "decode parameters must be positive") {
			t.Errorf("Wrong error message: %v", err)
		}
	})

	t.Run("negative_delta", func(t *testing.T) {
		configData := createTestConfig()
		env := createTestEnvironment()

		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("NewTuner failed: %v", err)
		}

		results := &TunedResults{
			ServiceParms: &analyzer.ServiceParms{
				Decode: &analyzer.DecodeParms{
					Alpha: 5.0,
					Beta:  2.5,
				},
				Prefill: &analyzer.PrefillParms{
					Gamma: 10.0,
					Delta: -0.15, // Negative
				},
			},
		}

		err = tuner.validateTunedResults(results)
		if err == nil {
			t.Error("Expected error for negative delta")
		}
		if err != nil && !strings.Contains(err.Error(), "prefill parameters must be positive") {
			t.Errorf("Wrong error message: %v", err)
		}
	})

	t.Run("zero_alpha", func(t *testing.T) {
		configData := createTestConfig()
		env := createTestEnvironment()

		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("NewTuner failed: %v", err)
		}

		results := &TunedResults{
			ServiceParms: &analyzer.ServiceParms{
				Decode: &analyzer.DecodeParms{
					Alpha: 0.0, // Zero
					Beta:  2.5,
				},
				Prefill: &analyzer.PrefillParms{
					Gamma: 10.0,
					Delta: 0.15,
				},
			},
		}

		err = tuner.validateTunedResults(results)
		if err == nil {
			t.Error("Expected error for zero alpha")
		}
		if err != nil && !strings.Contains(err.Error(), "decode parameters must be positive") {
			t.Errorf("Wrong error message: %v", err)
		}
	})

	t.Run("singular_innovation_covariance", func(t *testing.T) {
		configData := createTestConfig()
		env := createTestEnvironment()

		tuner, err := NewTuner(configData, env)
		if err != nil {
			t.Fatalf("NewTuner failed: %v", err)
		}

		// Create results with valid parameters
		results := &TunedResults{
			ServiceParms: &analyzer.ServiceParms{
				Decode: &analyzer.DecodeParms{
					Alpha: 5.0,
					Beta:  2.5,
				},
				Prefill: &analyzer.PrefillParms{
					Gamma: 10.0,
					Delta: 0.15,
				},
			},
		}

		// Set innovation to valid values
		tuner.filter.Y = mat.NewVecDense(2, []float64{1.0, 1.0})

		// Create a singular S matrix (all zeros)
		singularS := mat.NewDense(2, 2, []float64{0, 0, 0, 0})
		tuner.filter.S = singularS

		err = tuner.validateTunedResults(results)
		if err == nil {
			t.Error("Expected error for singular innovation covariance matrix")
		}
		if err != nil && !strings.Contains(err.Error(), "singular innovation covariance") {
			t.Errorf("Wrong error message: %v", err)
		}
	})
}

func TestTuner_ExtractTunedResultsVerification(t *testing.T) {
	configData := createTestConfig()
	env := createTestEnvironment()

	tuner, err := NewTuner(configData, env)
	if err != nil {
		t.Fatalf("NewTuner failed: %v", err)
	}

	// Get initial state
	initialAlpha := tuner.X().AtVec(StateIndexAlpha)

	// Extract results
	results, err := tuner.extractTunedResults()
	if err != nil {
		t.Fatalf("extractTunedResults failed: %v", err)
	}

	// Verify state wasn't modified
	currentAlpha := tuner.X().AtVec(StateIndexAlpha)
	if currentAlpha != initialAlpha {
		t.Error("extractTunedResults modified the tuner state")
	}

	// Verify extracted values match state
	if float64(results.ServiceParms.Decode.Alpha) != initialAlpha {
		t.Error("Extracted alpha doesn't match tuner state")
	}

	// Verify all results fields are populated
	if results.ServiceParms == nil {
		t.Error("ServiceParms is nil")
	}
	if results.Innovation == nil {
		t.Error("Innovation is nil")
	}
	if results.Covariance == nil {
		t.Error("Covariance is nil")
	}
}
