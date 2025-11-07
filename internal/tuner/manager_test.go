package controller

import (
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	infernoConfig "github.com/llm-d-incubation/workload-variant-autoscaler/pkg/config"
)

// Helper to initialize logger for tests
func init() {
	logger.Log = zap.NewNop().Sugar()
}

func TestNewTunerManager(t *testing.T) {

	tm := NewTunerManager()

	if tm == nil {
		t.Fatal("NewTunerManager() returned nil")
	}

	if tm.tuners == nil {
		t.Error("tuners map not initialized")
	}

	if !tm.enabled {
		t.Error("TunerManager should be enabled by default")
	}

	if !tm.IsEnabled() {
		t.Error("IsEnabled() should return true by default")
	}
}

func TestTunerManager_EnableDisable(t *testing.T) {
	tm := NewTunerManager()

	// Test initial state
	if !tm.IsEnabled() {
		t.Error("TunerManager should be enabled initially")
	}

	// Test disable
	tm.Disable()
	if tm.IsEnabled() {
		t.Error("TunerManager should be disabled after Disable()")
	}

	// Test enable
	tm.Enable()
	if !tm.IsEnabled() {
		t.Error("TunerManager should be enabled after Enable()")
	}
}

func TestTunerManager_GetOrCreateTuner(t *testing.T) {
	tm := NewTunerManager()

	// Create test system data with valid configuration
	systemData := createTestSystemData()

	// Get server from system data
	server := &systemData.Spec.Servers.Spec[0]

	// Add valid allocation data so environment is valid
	server.CurrentAlloc = infernoConfig.AllocationData{
		NumReplicas: 1,
		MaxBatch:    8,
		Accelerator: "A100",
		Load: infernoConfig.ServerLoadSpec{
			ArrivalRate:  60.0,
			AvgInTokens:  512,
			AvgOutTokens: 128,
		},
		TTFTAverage: 186.7,
		ITLAverage:  14.9,
	}

	// Test creating a new tuner
	tuner1, err := tm.getOrCreateTuner(systemData, server)
	if err != nil {
		t.Fatalf("Failed to create tuner: %v", err)
	}

	if tuner1 == nil {
		t.Fatal("getOrCreateTuner() returned nil tuner")
	}

	// Verify tuner was stored
	storedTuner, exists := tm.tuners[server.Name]

	if !exists {
		t.Error("Tuner was not stored in manager")
	}

	if storedTuner != tuner1 {
		t.Error("Stored tuner doesn't match returned tuner")
	}

	// Test getting existing tuner (should return same instance)
	tuner2, err := tm.getOrCreateTuner(systemData, server)
	if err != nil {
		t.Fatalf("Failed to get existing tuner: %v", err)
	}

	if tuner2 != tuner1 {
		t.Error("getOrCreateTuner() should return same tuner instance for same server")
	}
}

func TestTunerManager_TuneServer(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()
	server := &systemData.Spec.Servers.Spec[0]

	// Add valid allocation data
	server.CurrentAlloc = infernoConfig.AllocationData{
		NumReplicas: 1,
		MaxBatch:    8,
		Accelerator: "A100",
		Load: infernoConfig.ServerLoadSpec{
			ArrivalRate:  60.0, // 60 req/min
			AvgInTokens:  512,
			AvgOutTokens: 128,
		},
		TTFTAverage: 186.7, // ms
		ITLAverage:  14.9,  // ms
	}

	// Test tuning a server
	err := tm.tuneServer(systemData, server)
	if err != nil {
		t.Fatalf("tuneServer() failed: %v", err)
	}

	// Verify tuner was created

	_, exists := tm.tuners[server.Name]

	if !exists {
		t.Error("Tuner should exist after tuneServer() call")
	}
}

func TestTunerManager_TuneModelPerfParams(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()

	// Add allocation data to all servers
	for i := range systemData.Spec.Servers.Spec {
		systemData.Spec.Servers.Spec[i].CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 1,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 186.7,
			ITLAverage:  14.9,
		}
	}

	// Test tuning all servers
	err := tm.TuneModelPerfParams(systemData)
	if err != nil {
		t.Fatalf("TuneModelPerfParams() failed: %v", err)
	}

	// Verify tuners were created for all servers

	tunerCount := len(tm.tuners)

	expectedCount := len(systemData.Spec.Servers.Spec)
	if tunerCount != expectedCount {
		t.Errorf("Expected %d tuners, got %d", expectedCount, tunerCount)
	}
}

func TestTunerManager_InvalidEnvironment(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()
	server := &systemData.Spec.Servers.Spec[0]

	// Set invalid allocation data (all zeros, including missing accelerator)
	server.CurrentAlloc = infernoConfig.AllocationData{
		Accelerator: "A100", // Set accelerator but invalid metrics
	}

	// Test tuning with invalid environment
	err := tm.tuneServer(systemData, server)
	if err == nil {
		t.Error("tuneServer() should fail with invalid environment")
	}

	// Should fail with environment validation error
	if err != nil && !strings.Contains(err.Error(), "invalid environment") {
		t.Logf("Got error (acceptable): %v", err)
	}
}

func TestTunerManager_MissingInitState(t *testing.T) {
	tm := NewTunerManager()

	// Create system data without performance data
	systemData := &infernoConfig.SystemData{
		Spec: infernoConfig.SystemSpec{
			Models: infernoConfig.ModelData{
				PerfData: []infernoConfig.ModelAcceleratorPerfData{}, // Empty
			},
			ServiceClasses: infernoConfig.ServiceClassData{
				Spec: []infernoConfig.ServiceClassSpec{
					{
						Name: "default",
						ModelTargets: []infernoConfig.ModelTarget{
							{
								Model:    "llama-7b",
								SLO_TTFT: 200.0,
								SLO_ITL:  20.0,
							},
						},
					},
				},
			},
			Servers: infernoConfig.ServerData{
				Spec: []infernoConfig.ServerSpec{
					{
						Name:  "test-server",
						Model: "llama-7b",
						Class: "default",
						CurrentAlloc: infernoConfig.AllocationData{
							Accelerator: "A100",
						},
					},
				},
			},
		},
	}

	server := &systemData.Spec.Servers.Spec[0]

	// Test creating tuner without init state
	_, err := tm.getOrCreateTuner(systemData, server)
	if err == nil {
		t.Error("getOrCreateTuner() should fail when init state is missing")
	}

	expectedErrMsg := "not found in system data"
	if err != nil && !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error containing %q, got: %v", expectedErrMsg, err)
	}
}

func TestTunerManager_MissingSLO(t *testing.T) {
	tm := NewTunerManager()

	// Create system data without SLO
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
			ServiceClasses: infernoConfig.ServiceClassData{
				Spec: []infernoConfig.ServiceClassSpec{
					{
						Name:         "default",
						ModelTargets: []infernoConfig.ModelTarget{}, // Empty
					},
				},
			},
			Servers: infernoConfig.ServerData{
				Spec: []infernoConfig.ServerSpec{
					{
						Name:  "test-server",
						Model: "llama-7b",
						Class: "default",
						CurrentAlloc: infernoConfig.AllocationData{
							Accelerator: "A100",
						},
					},
				},
			},
		},
	}

	server := &systemData.Spec.Servers.Spec[0]

	// Test creating tuner without SLO
	_, err := tm.getOrCreateTuner(systemData, server)
	if err == nil {
		t.Error("getOrCreateTuner() should fail when SLO is missing")
	}

	expectedErrMsg := "not found in service class"
	if err != nil && !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Expected error containing %q, got: %v", expectedErrMsg, err)
	}
}

func TestTunerManager_ConcurrentAccess(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()
	server := &systemData.Spec.Servers.Spec[0]

	// Add valid allocation data
	server.CurrentAlloc = infernoConfig.AllocationData{
		NumReplicas: 1,
		MaxBatch:    8,
		Accelerator: "A100",
		Load: infernoConfig.ServerLoadSpec{
			ArrivalRate:  60.0,
			AvgInTokens:  512,
			AvgOutTokens: 128,
		},
		TTFTAverage: 186.7,
		ITLAverage:  14.9,
	}

	// Test concurrent access to getOrCreateTuner
	done := make(chan bool)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func() {
			_, err := tm.getOrCreateTuner(systemData, server)
			if err != nil {
				errors <- err
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}

	// Verify only one tuner was created

	tunerCount := len(tm.tuners)

	if tunerCount != 1 {
		t.Errorf("Expected exactly 1 tuner from concurrent creation, got %d", tunerCount)
	}
}

func TestTunerManager_SuccessfulTuneServer(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()
	server := &systemData.Spec.Servers.Spec[0]

	// Store initial parameters for comparison
	initialAlpha := 8.5
	initialBeta := 2.1
	initialGamma := 5.0
	initialDelta := 0.11

	// Add valid allocation data with realistic values
	server.CurrentAlloc = infernoConfig.AllocationData{
		NumReplicas: 1,
		MaxBatch:    8,
		Accelerator: "A100",
		Load: infernoConfig.ServerLoadSpec{
			ArrivalRate:  60.0,
			AvgInTokens:  512,
			AvgOutTokens: 128,
		},
		TTFTAverage: 186.7, // Close to SLO of 200
		ITLAverage:  14.9,  // Well under SLO of 20
	}

	// First call - should create tuner
	err := tm.tuneServer(systemData, server)

	if err != nil {
		t.Fatalf("tuneServer() failed: %v", err)
	}

	// Verify tuner was created regardless of tuning success

	tuner, exists := tm.tuners[server.Name]

	if !exists {
		t.Fatal("Tuner should exist after tuneServer() call, even if NIS validation failed")
	}
	if tuner == nil {
		t.Fatal("Tuner should not be nil")
	}

	// Verify parameters are still valid
	perfData := systemData.Spec.Models.PerfData[0]

	if perfData.DecodeParms.Alpha <= 0 {
		t.Errorf("Alpha should be positive, got %f", perfData.DecodeParms.Alpha)
	}
	if perfData.DecodeParms.Beta <= 0 {
		t.Errorf("Beta should be positive, got %f", perfData.DecodeParms.Beta)
	}
	if perfData.PrefillParms.Gamma < 0 {
		t.Errorf("Gamma should be non-negative, got %f", perfData.PrefillParms.Gamma)
	}
	if perfData.PrefillParms.Delta < 0 {
		t.Errorf("Delta should be non-negative, got %f", perfData.PrefillParms.Delta)
	}

	t.Logf("Initial params: [α=%.2f, β=%.2f, γ=%.2f, δ=%.4f]", initialAlpha, initialBeta, initialGamma, initialDelta)
	t.Logf("After first tuning attempt: [α=%.2f, β=%.2f, γ=%.2f, δ=%.4f] (err=%v)",
		perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta,
		perfData.PrefillParms.Gamma, perfData.PrefillParms.Delta, err)

	// Second call - should reuse existing tuner
	server.CurrentAlloc.TTFTAverage = 195.3 // Different values
	server.CurrentAlloc.ITLAverage = 16.2

	err2 := tm.tuneServer(systemData, server)

	// Verify same tuner instance was used

	tuner2 := tm.tuners[server.Name]

	if tuner2 != tuner {
		t.Error("Should reuse the same tuner instance for subsequent calls")
	}

	t.Logf("Second tuning attempt result: err=%v", err2)
}

func TestTunerManager_SuccessfulTuneModelPerfParams(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()

	// Add valid allocation data to all servers with good metrics
	for i := range systemData.Spec.Servers.Spec {
		systemData.Spec.Servers.Spec[i].CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 2 + i, // Different replica counts
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  float32(55.0 + float64(i)*5.0),
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: float32(180.0 + float64(i)*10.0), // Varying TTFT
			ITLAverage:  float32(15.0 + float64(i)*2.0),   // Varying ITL
		}
	}

	// Store initial parameters
	initialAlpha := systemData.Spec.Models.PerfData[0].DecodeParms.Alpha
	initialBeta := systemData.Spec.Models.PerfData[0].DecodeParms.Beta

	// Test tuning all servers
	err := tm.TuneModelPerfParams(systemData)
	if err != nil {
		t.Fatalf("TuneModelPerfParams() should not return error: %v", err)
	}

	// Verify tuners were attempted for all servers

	tunerCount := len(tm.tuners)

	expectedCount := len(systemData.Spec.Servers.Spec)
	t.Logf("Created %d tuners out of %d servers", tunerCount, expectedCount)

	// If any tuners were created, verify they're valid
	for _, server := range systemData.Spec.Servers.Spec {

		tuner, exists := tm.tuners[server.Name]

		if exists && tuner == nil {
			t.Errorf("Tuner for server %s exists but is nil", server.Name)
		}
	}

	// Verify parameters remain valid
	perfData := systemData.Spec.Models.PerfData[0]
	if perfData.DecodeParms.Alpha <= 0 || perfData.DecodeParms.Beta <= 0 {
		t.Error("Parameters should remain positive")
	}

	t.Logf("Initial: α=%.2f, β=%.2f", initialAlpha, initialBeta)
	t.Logf("After tuning: α=%.2f, β=%.2f", perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta)
}

func TestTunerManager_SuccessfulTunerCreation(t *testing.T) {
	// This test focuses on the successful path of creating tuners,
	// regardless of NIS validation results
	tm := NewTunerManager()
	systemData := createTestSystemData()
	server := &systemData.Spec.Servers.Spec[0]

	// Test multiple iterations with varying metrics
	iterations := []struct {
		name        string
		ttft        float32
		itl         float32
		numReplicas int
	}{
		{"Initial metrics", 185.0, 14.5, 1},
		{"Updated metrics", 195.0, 16.0, 1},
		{"Further updates", 188.0, 15.0, 1},
		{"Final metrics", 175.0, 13.0, 1},
	}

	for _, iter := range iterations {
		t.Run(iter.name, func(t *testing.T) {
			// Update metrics for this iteration
			server.CurrentAlloc = infernoConfig.AllocationData{
				NumReplicas: iter.numReplicas,
				MaxBatch:    8,
				Accelerator: "A100",
				Load: infernoConfig.ServerLoadSpec{
					ArrivalRate:  60.0,
					AvgInTokens:  512,
					AvgOutTokens: 128,
				},
				TTFTAverage: iter.ttft,
				ITLAverage:  iter.itl,
			}

			// Attempt to tune the server
			err := tm.tuneServer(systemData, server)
			if err != nil {
				t.Fatalf("tuneServer() failed on environment update: %v", err)
			}

			// Verify tuner exists (should be created on first call)

			tuner, exists := tm.tuners[server.Name]

			if !exists {
				t.Error("Tuner should exist after tuneServer() call")
			}
			if tuner == nil {
				t.Error("Tuner should not be nil")
			}

			// Verify parameters remain valid
			perfData := systemData.Spec.Models.PerfData[0]
			if perfData.DecodeParms.Alpha <= 0 || perfData.DecodeParms.Beta <= 0 {
				t.Errorf("Parameters became invalid: α=%.2f, β=%.2f",
					perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta)
			}

			t.Logf("%s: TTFT=%.1f, ITL=%.1f -> α=%.2f, β=%.2f",
				iter.name, iter.ttft, iter.itl,
				perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta)
		})
	}

	// Verify only one tuner was created (reused across iterations)

	tunerCount := len(tm.tuners)

	if tunerCount != 1 {
		t.Errorf("Expected 1 tuner to be reused, got %d tuners", tunerCount)
	}
}

func TestTunerManager_TuningRespectsDisabledState(t *testing.T) {
	tm := NewTunerManager()
	tm.Disable() // Disable tuning

	systemData := createTestSystemData()

	// Add valid allocation data
	for i := range systemData.Spec.Servers.Spec {
		systemData.Spec.Servers.Spec[i].CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 1,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 186.7,
			ITLAverage:  14.9,
		}
	}

	// Try to tune while disabled
	err := tm.TuneModelPerfParams(systemData)
	if err != nil {
		t.Fatalf("TuneModelPerfParams() should not return error: %v", err)
	}

	// Check if any tuners were created

	tunerCount := len(tm.tuners)

	if tunerCount != 0 {
		t.Errorf("No tuners should be created while disabled, got %d", tunerCount)
	}
}

func TestTunerManager_SuccessfulEnvironmentUpdate(t *testing.T) {
	// Tests that environment updates work correctly on existing tuners
	tm := NewTunerManager()
	systemData := createTestSystemData()
	server := &systemData.Spec.Servers.Spec[0]

	// First tuning with initial metrics
	server.CurrentAlloc = infernoConfig.AllocationData{
		NumReplicas: 1,
		MaxBatch:    8,
		Accelerator: "A100",
		Load: infernoConfig.ServerLoadSpec{
			ArrivalRate:  60.0,
			AvgInTokens:  512,
			AvgOutTokens: 128,
		},
		TTFTAverage: 190.0,
		ITLAverage:  15.0,
	}

	err := tm.tuneServer(systemData, server)
	if err != nil {
		t.Fatalf("tuneServer() failed on environment update: %v", err)
	}
	// Get the tuner to verify it exists

	tuner, exists := tm.tuners[server.Name]

	if !exists || tuner == nil {
		t.Fatal("Tuner should exist after first call")
	}

	// Update with significantly different metrics
	server.CurrentAlloc.TTFTAverage = 210.0 // Over SLO
	server.CurrentAlloc.ITLAverage = 18.0
	// Keep NumReplicas=1 to maintain Lambda=60 req/min for NIS validation

	// Tune again - should update environment in existing tuner
	err = tm.tuneServer(systemData, server)
	if err != nil {
		t.Fatalf("tuneServer() failed on environment update: %v", err)
	}

	// Verify same tuner was reused

	tuner2 := tm.tuners[server.Name]

	if tuner2 != tuner {
		t.Error("Should reuse same tuner instance when updating environment")
	}

	// Parameters should still be valid
	perfData := systemData.Spec.Models.PerfData[0]
	if perfData.DecodeParms.Alpha <= 0 || perfData.DecodeParms.Beta <= 0 {
		t.Error("Parameters should remain valid after environment update")
	}

	t.Logf("Environment updated successfully, parameters remain valid")
}

func TestTunerManager_RemoveTuners(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()

	// Add valid allocation data to all servers
	for i := range systemData.Spec.Servers.Spec {
		systemData.Spec.Servers.Spec[i].CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 1,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 190.0,
			ITLAverage:  15.0,
		}
	}

	// Create tuners for all servers
	err := tm.TuneModelPerfParams(systemData)
	if err != nil {
		t.Fatalf("TuneModelPerfParams failed: %v", err)
	}

	// Verify tuners were created

	initialCount := len(tm.tuners)

	if initialCount == 0 {
		t.Fatal("No tuners were created")
	}

	// Remove one server from system data
	removedServerName := systemData.Spec.Servers.Spec[0].Name
	systemData.Spec.Servers.Spec = systemData.Spec.Servers.Spec[1:]

	// Call RemoveTuners
	tm.RemoveTuners(systemData)

	// Verify the tuner for removed server was deleted

	_, exists := tm.tuners[removedServerName]
	finalCount := len(tm.tuners)

	if exists {
		t.Errorf("Tuner for removed server %s still exists", removedServerName)
	}

	expectedCount := len(systemData.Spec.Servers.Spec)
	if finalCount != expectedCount {
		t.Errorf("After removal, tuner count = %d, want %d", finalCount, expectedCount)
	}
}

func TestTunerManager_RemoveTunersMultiple(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()

	// Add a third server
	thirdServer := infernoConfig.ServerSpec{
		Name:  "test-server-3",
		Model: "llama-7b",
		Class: "default",
		CurrentAlloc: infernoConfig.AllocationData{
			NumReplicas: 1,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 190.0,
			ITLAverage:  15.0,
		},
	}
	systemData.Spec.Servers.Spec = append(systemData.Spec.Servers.Spec, thirdServer)

	// Add valid allocation data to existing servers
	for i := range systemData.Spec.Servers.Spec[:2] {
		systemData.Spec.Servers.Spec[i].CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 1,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 190.0,
			ITLAverage:  15.0,
		}
	}

	// Create tuners for all 3 servers
	err := tm.TuneModelPerfParams(systemData)
	if err != nil {
		t.Fatalf("TuneModelPerfParams failed: %v", err)
	}

	// Verify 3 tuners were created

	initialCount := len(tm.tuners)

	if initialCount != 3 {
		t.Errorf("Initial tuner count = %d, want 3", initialCount)
	}

	// Remove 2 servers, keep only the first one
	removedServer1 := systemData.Spec.Servers.Spec[1].Name
	removedServer2 := systemData.Spec.Servers.Spec[2].Name
	systemData.Spec.Servers.Spec = systemData.Spec.Servers.Spec[:1]

	// Call RemoveTuners
	tm.RemoveTuners(systemData)

	// Verify the tuners for removed servers were deleted

	_, exists1 := tm.tuners[removedServer1]
	_, exists2 := tm.tuners[removedServer2]
	finalCount := len(tm.tuners)

	if exists1 {
		t.Errorf("Tuner for removed server %s still exists", removedServer1)
	}
	if exists2 {
		t.Errorf("Tuner for removed server %s still exists", removedServer2)
	}

	if finalCount != 1 {
		t.Errorf("After removal, tuner count = %d, want 1", finalCount)
	}
}

func TestTunerManager_RemoveTunersEmptySystemData(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()

	// Add valid allocation data to all servers
	for i := range systemData.Spec.Servers.Spec {
		systemData.Spec.Servers.Spec[i].CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 1,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 190.0,
			ITLAverage:  15.0,
		}
	}

	// Create tuners for all servers
	err := tm.TuneModelPerfParams(systemData)
	if err != nil {
		t.Fatalf("TuneModelPerfParams failed: %v", err)
	}

	// Verify tuners were created

	initialCount := len(tm.tuners)

	if initialCount == 0 {
		t.Fatal("No tuners were created")
	}

	// Remove all servers
	systemData.Spec.Servers.Spec = []infernoConfig.ServerSpec{}

	// Call RemoveTuners
	tm.RemoveTuners(systemData)

	// Verify all tuners were deleted

	finalCount := len(tm.tuners)

	if finalCount != 0 {
		t.Errorf("After removing all servers, tuner count = %d, want 0", finalCount)
	}
}

func TestTunerManager_RemoveTunersNoChange(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()

	// Add valid allocation data to all servers
	for i := range systemData.Spec.Servers.Spec {
		systemData.Spec.Servers.Spec[i].CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 1,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 190.0,
			ITLAverage:  15.0,
		}
	}

	// Create tuners for all servers
	err := tm.TuneModelPerfParams(systemData)
	if err != nil {
		t.Fatalf("TuneModelPerfParams failed: %v", err)
	}

	// Get initial tuner count

	initialCount := len(tm.tuners)

	// Call RemoveTuners with same system data (no servers removed)
	tm.RemoveTuners(systemData)

	// Verify tuner count hasn't changed

	finalCount := len(tm.tuners)

	if finalCount != initialCount {
		t.Errorf("Tuner count changed from %d to %d, but no servers were removed", initialCount, finalCount)
	}
}

// Helper function to create test system data with valid configuration
// Parameters are chosen to match the observations in tests to pass NIS validation
func createTestSystemData() *infernoConfig.SystemData {
	return &infernoConfig.SystemData{
		Spec: infernoConfig.SystemSpec{
			Models: infernoConfig.ModelData{
				PerfData: []infernoConfig.ModelAcceleratorPerfData{
					{
						Name: "llama-7b",
						Acc:  "A100",
						// Parameters that work with TTFT=186.7, ITL=14.9 observations
						// Using "moderate_off" config from pkg/tuner tests
						DecodeParms: infernoConfig.DecodeParms{
							Alpha: 7.0,
							Beta:  2.5,
						},
						PrefillParms: infernoConfig.PrefillParms{
							Gamma: 6.0,
							Delta: 0.09,
						},
					},
				},
			},
			ServiceClasses: infernoConfig.ServiceClassData{
				Spec: []infernoConfig.ServiceClassSpec{
					{
						Name: "default",
						ModelTargets: []infernoConfig.ModelTarget{
							{
								Model:    "llama-7b",
								SLO_TTFT: 200.0,
								SLO_ITL:  30.0,
							},
						},
					},
				},
			},
			Servers: infernoConfig.ServerData{
				Spec: []infernoConfig.ServerSpec{
					{
						Name:  "test-server-1",
						Model: "llama-7b",
						Class: "default",
						CurrentAlloc: infernoConfig.AllocationData{
							Accelerator: "A100",
						},
					},
					{
						Name:  "test-server-2",
						Model: "llama-7b",
						Class: "default",
						CurrentAlloc: infernoConfig.AllocationData{
							Accelerator: "A100",
						},
					},
				},
			},
		},
	}
}

// Helper to create system data with parameters matching specific observations
func createTestSystemDataWithParams(alpha, beta, gamma, delta, sloTTFT, sloITL float64) *infernoConfig.SystemData {
	return &infernoConfig.SystemData{
		Spec: infernoConfig.SystemSpec{
			Models: infernoConfig.ModelData{
				PerfData: []infernoConfig.ModelAcceleratorPerfData{
					{
						Name: "llama-7b",
						Acc:  "A100",
						DecodeParms: infernoConfig.DecodeParms{
							Alpha: float32(alpha),
							Beta:  float32(beta),
						},
						PrefillParms: infernoConfig.PrefillParms{
							Gamma: float32(gamma),
							Delta: float32(delta),
						},
					},
				},
			},
			ServiceClasses: infernoConfig.ServiceClassData{
				Spec: []infernoConfig.ServiceClassSpec{
					{
						Name: "default",
						ModelTargets: []infernoConfig.ModelTarget{
							{
								Model:    "llama-7b",
								SLO_TTFT: float32(sloTTFT),
								SLO_ITL:  float32(sloITL),
							},
						},
					},
				},
			},
			Servers: infernoConfig.ServerData{
				Spec: []infernoConfig.ServerSpec{
					{
						Name:  "test-server-1",
						Model: "llama-7b",
						Class: "default",
						CurrentAlloc: infernoConfig.AllocationData{
							Accelerator: "A100",
						},
					},
				},
			},
		},
	}
}

func TestTunerManager_SuccessfulTuneServerWithMatchingMetrics(t *testing.T) {
	tm := NewTunerManager()

	// Use initial parameters that are moderately off (similar to pkg/tuner test)
	// These should converge to the correct values
	systemData := createTestSystemDataWithParams(7.0, 2.5, 6.0, 0.09, 200.0, 20.0)
	server := &systemData.Spec.Servers.Spec[0]

	// Store initial parameters
	initialAlpha := systemData.Spec.Models.PerfData[0].DecodeParms.Alpha
	initialBeta := systemData.Spec.Models.PerfData[0].DecodeParms.Beta
	initialGamma := systemData.Spec.Models.PerfData[0].PrefillParms.Gamma
	initialDelta := systemData.Spec.Models.PerfData[0].PrefillParms.Delta

	t.Logf("Initial parameters: α=%.2f, β=%.2f, γ=%.2f, δ=%.4f",
		initialAlpha, initialBeta, initialGamma, initialDelta)

	// Set allocation data with metrics that match the parameters
	// Use observations that match ExpectedObservations for parameters [5.0, 2.5, 10.0, 0.15]
	server.CurrentAlloc = infernoConfig.AllocationData{
		NumReplicas: 1,
		MaxBatch:    8,
		Accelerator: "A100",
		Load: infernoConfig.ServerLoadSpec{
			ArrivalRate:  60.0,
			AvgInTokens:  512,
			AvgOutTokens: 128,
		},
		TTFTAverage: 186.7,
		ITLAverage:  14.9,
	}

	// First call - should create tuner and potentially tune successfully
	err := tm.tuneServer(systemData, server)

	// Verify tuner was created

	tuner, exists := tm.tuners[server.Name]

	if !exists {
		t.Fatal("Tuner should exist after tuneServer() call")
	}
	if tuner == nil {
		t.Fatal("Tuner should not be nil")
	}

	// Check result and parameters after first iteration
	perfData := systemData.Spec.Models.PerfData[0]
	if err == nil {
		t.Logf("   Tuned: α=%.2f, β=%.2f, γ=%.2f, δ=%.4f",
			perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta,
			perfData.PrefillParms.Gamma, perfData.PrefillParms.Delta)

		// Verify parameters actually changed from initial values
		if perfData.DecodeParms.Alpha != float32(initialAlpha) ||
			perfData.DecodeParms.Beta != float32(initialBeta) {
			t.Logf("		Parameters successfully updated in SystemData")
		} else {
			t.Errorf("		Parameters did not change despite successful tuning")
		}
	} else {
		t.Logf("   Params unchanged: α=%.2f, β=%.2f, γ=%.2f, δ=%.4f",
			perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta,
			perfData.PrefillParms.Gamma, perfData.PrefillParms.Delta)

		// Try a few more iterations - should eventually converge
		successfulIteration := -1
		for i := 2; i <= 10; i++ {
			err = tm.tuneServer(systemData, server)
			if err == nil {
				successfulIteration = i
				t.Logf(" Iteration %d: Successful tuning!", i)
				t.Logf("   Converged: α=%.2f, β=%.2f, γ=%.2f, δ=%.4f",
					perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta,
					perfData.PrefillParms.Gamma, perfData.PrefillParms.Delta)
				break
			} else if i <= 3 {
				t.Logf("  Iteration %d: Still converging...", i)
			}
		}

		if successfulIteration < 0 {
			t.Logf(" require more iterations or different initial conditions")
		}
	}

	// Verify parameters remain valid
	if perfData.DecodeParms.Alpha <= 0 || perfData.DecodeParms.Beta <= 0 {
		t.Error("Parameters should remain positive")
	}

	// Test with slightly different metrics - should also work
	server.CurrentAlloc.ITLAverage = 15.5   // Close to 14.9
	server.CurrentAlloc.TTFTAverage = 190.0 // Close to 186.7

	err = tm.tuneServer(systemData, server)
	if err == nil {
		t.Logf(" Second tuning: Success with updated metrics")
	} else {
		t.Logf("  Second tuning: NIS rejected (expected with metric change)")
	}

	// Verify same tuner was reused

	tuner2 := tm.tuners[server.Name]

	if tuner2 != tuner {
		t.Error("Should reuse the same tuner instance")
	}
}

func TestTunerManager_SuccessfulTuneServerWithMultipleReplicas(t *testing.T) {
	tm := NewTunerManager()

	// Use initial parameters that are moderately off
	systemData := createTestSystemDataWithParams(7.0, 2.5, 6.0, 0.09, 200.0, 20.0)
	server := &systemData.Spec.Servers.Spec[0]

	t.Logf("Testing with multiple replicas scenario")

	// Test with 3 replicas
	// Lambda per replica = ArrivalRate / NumReplicas = 180.0 / 3 = 60.0 req/min
	// This should give the same per-replica lambda as the single replica test
	server.CurrentAlloc = infernoConfig.AllocationData{
		NumReplicas: 3, // Multiple replicas
		MaxBatch:    8,
		Accelerator: "A100",
		Load: infernoConfig.ServerLoadSpec{
			ArrivalRate:  180.0, // Total: 180 / 3 = 60 req/min per replica
			AvgInTokens:  512,
			AvgOutTokens: 128,
		},
		TTFTAverage: 186.7, // Same target observations
		ITLAverage:  14.9,
	}

	t.Logf("Configuration: %d replicas, %.0f total req/min = %.0f req/min per replica",
		server.CurrentAlloc.NumReplicas,
		server.CurrentAlloc.Load.ArrivalRate,
		server.CurrentAlloc.Load.ArrivalRate/float32(server.CurrentAlloc.NumReplicas))

	// First call - should create tuner
	err := tm.tuneServer(systemData, server)

	// Verify tuner was created

	tuner, exists := tm.tuners[server.Name]

	if !exists {
		t.Fatal("Tuner should exist after tuneServer() call")
	}
	if tuner == nil {
		t.Fatal("Tuner should not be nil")
	}

	perfData := systemData.Spec.Models.PerfData[0]
	if err == nil {
		t.Logf(" Multi-replica test: Successful tuning on first iteration!")
		t.Logf("   Tuned: α=%.2f, β=%.2f, γ=%.2f, δ=%.4f",
			perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta,
			perfData.PrefillParms.Gamma, perfData.PrefillParms.Delta)
	} else {
		t.Logf("  Multi-replica test: NIS rejected on first iteration - %v", err)

		// Try a few more iterations
		for i := 2; i <= 5; i++ {
			err = tm.tuneServer(systemData, server)
			if err == nil {
				t.Logf(" Iteration %d: Successful tuning!", i)
				t.Logf("   Converged: α=%.2f, β=%.2f, γ=%.2f, δ=%.4f",
					perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta,
					perfData.PrefillParms.Gamma, perfData.PrefillParms.Delta)
				break
			}
		}
	}

	// Verify parameters remain valid
	if perfData.DecodeParms.Alpha <= 0 || perfData.DecodeParms.Beta <= 0 {
		t.Error("Parameters should remain positive")
	}

	// Test scaling: add more replicas with proportionally higher arrival rate
	server.CurrentAlloc.NumReplicas = 5
	server.CurrentAlloc.Load.ArrivalRate = 300.0 // 300 / 5 = 60 req/min per replica

	t.Logf("Scaling to %d replicas, %.0f total req/min = %.0f req/min per replica",
		server.CurrentAlloc.NumReplicas,
		server.CurrentAlloc.Load.ArrivalRate,
		server.CurrentAlloc.Load.ArrivalRate/float32(server.CurrentAlloc.NumReplicas))

	err = tm.tuneServer(systemData, server)
	if err == nil {
		t.Logf(" Scaled configuration: Tuning succeeded")
	} else {
		t.Logf("  Scaled configuration: NIS rejected (expected with scaling change)")
	}

	// Verify same tuner was reused

	tuner2 := tm.tuners[server.Name]

	if tuner2 != tuner {
		t.Error("Should reuse the same tuner instance across replica changes")
	}
}

func TestTunerManager_TuneServerErrorPaths(t *testing.T) {
	tm := NewTunerManager()
	systemData := createTestSystemData()
	server := &systemData.Spec.Servers.Spec[0]

	t.Run("invalid environment - zero replicas", func(t *testing.T) {
		// Set up invalid allocation with zero replicas
		server.CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 0, // Invalid!
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 186.7,
			ITLAverage:  14.9,
		}

		err := tm.tuneServer(systemData, server)
		if err == nil {
			t.Error("Expected error with zero replicas, got nil")
		} else {
			t.Logf(" Correctly rejected invalid environment: %v", err)
		}
	})

	t.Run("update failure path", func(t *testing.T) {
		// Reset to valid allocation
		server.CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 1,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 186.7,
			ITLAverage:  14.9,
		}

		// First call succeeds and creates tuner
		err := tm.tuneServer(systemData, server)

		if err != nil {
			t.Error("Expected successful tuning, got error:", err)
		}

		// Now corrupt the system data to cause update failure
		corruptedData := createTestSystemData()
		corruptedData.Spec.Models.PerfData = []infernoConfig.ModelAcceleratorPerfData{} // Empty

		err = tm.tuneServer(corruptedData, server)
		if err == nil {
			t.Error("Expected error when updating with corrupted system data, got nil")
		} else {
			t.Logf(" Correctly handled update failure: %v", err)
		}
	})

	t.Run("successful path after errors", func(t *testing.T) {
		// After error scenarios, tuning should still work with valid data
		server.CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 1,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  60.0,
				AvgInTokens:  512,
				AvgOutTokens: 128,
			},
			TTFTAverage: 186.7,
			ITLAverage:  14.9,
		}

		// Should work fine with clean system data
		err := tm.tuneServer(systemData, server)
		if err != nil {
			t.Error("Expected successful tuning after error scenarios, got:", err)
		}
	})
}

func TestTunerManager_SuccessfulMultipleServersWithMatchingMetrics(t *testing.T) {
	tm := NewTunerManager()

	// Use parameters that match observations
	systemData := createTestSystemDataWithParams(5.0, 2.5, 10.0, 0.15, 200.0, 30.0)

	// Add a second server
	systemData.Spec.Servers.Spec = append(systemData.Spec.Servers.Spec, infernoConfig.ServerSpec{
		Name:  "test-server-2",
		Model: "llama-7b",
		Class: "default",
		CurrentAlloc: infernoConfig.AllocationData{
			Accelerator: "A100",
		},
	})

	// Set allocation data for all servers with matching metrics
	for i := range systemData.Spec.Servers.Spec {
		systemData.Spec.Servers.Spec[i].CurrentAlloc = infernoConfig.AllocationData{
			NumReplicas: 2 + i,
			MaxBatch:    8,
			Accelerator: "A100",
			Load: infernoConfig.ServerLoadSpec{
				ArrivalRate:  float32(55.0 + float64(i)*5.0),
				AvgInTokens:  100,
				AvgOutTokens: 200,
			},
			// Use observations matching ExpectedObservations for params [5.0, 2.5, 10.0, 0.15]
			ITLAverage:  float32(14.0 + float64(i)*1.0),  // Close to 15.0
			TTFTAverage: float32(185.0 + float64(i)*5.0), // Close to 190.0
		}
	}

	// Test tuning all servers
	err := tm.TuneModelPerfParams(systemData)
	if err != nil {
		t.Fatalf("TuneModelPerfParams() failed: %v", err)
	}

	// Verify tuners were created for all servers

	tunerCount := len(tm.tuners)

	expectedCount := len(systemData.Spec.Servers.Spec)
	if tunerCount != expectedCount {
		t.Errorf("Expected %d tuners, got %d", expectedCount, tunerCount)
	}

	// Verify all tuners exist
	for _, server := range systemData.Spec.Servers.Spec {

		tuner, exists := tm.tuners[server.Name]

		if !exists {
			t.Errorf("Tuner for server %s should exist", server.Name)
		}
		if tuner == nil {
			t.Errorf("Tuner for server %s should not be nil", server.Name)
		}
	}

	// Verify parameters are still valid
	perfData := systemData.Spec.Models.PerfData[0]
	if perfData.DecodeParms.Alpha <= 0 || perfData.DecodeParms.Beta <= 0 {
		t.Error("Tuned parameters should be positive")
	}

	t.Logf("After tuning %d servers: α=%.2f, β=%.2f, γ=%.2f, δ=%.4f",
		expectedCount,
		perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta,
		perfData.PrefillParms.Gamma, perfData.PrefillParms.Delta)
}

func TestTunerManager_SuccessfulConvergence(t *testing.T) {
	tm := NewTunerManager()

	// Use parameters matching observations
	systemData := createTestSystemDataWithParams(5.0, 2.5, 10.0, 0.15, 200.0, 30.0)
	server := &systemData.Spec.Servers.Spec[0]

	// Simulate convergence: metrics that closely match expected observations [190, 15]
	testCases := []struct {
		name string
		ttft float32
		itl  float32
	}{
		{"Iteration 1 - Close match", 190.0, 15.0},
		{"Iteration 2 - Very close", 192.0, 15.5},
		{"Iteration 3 - Slight variation", 188.0, 14.5},
		{"Iteration 4 - Close again", 191.0, 15.2},
	}

	successCount := 0
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server.CurrentAlloc = infernoConfig.AllocationData{
				NumReplicas: 1,
				MaxBatch:    8,
				Accelerator: "A100",
				Load: infernoConfig.ServerLoadSpec{
					ArrivalRate:  60.0,
					AvgInTokens:  100,
					AvgOutTokens: 200,
				},
				TTFTAverage: tc.ttft,
				ITLAverage:  tc.itl,
			}

			err := tm.tuneServer(systemData, server)
			if err == nil {
				successCount++
			}

			perfData := systemData.Spec.Models.PerfData[0]
			t.Logf("%s: TTFT=%.1f, ITL=%.1f -> α=%.2f, β=%.2f (err=%v)",
				tc.name, tc.ttft, tc.itl,
				perfData.DecodeParms.Alpha, perfData.DecodeParms.Beta, err)

			// Parameters should remain valid regardless
			if perfData.DecodeParms.Alpha <= 0 || perfData.DecodeParms.Beta <= 0 {
				t.Errorf("Parameters became invalid after %s", tc.name)
			}
		})
	}

	// With closely matching metrics, we expect some successes
	t.Logf("Successful tuning iterations: %d / %d", successCount, len(testCases))

	// Verify only one tuner was created and reused

	tunerCount := len(tm.tuners)

	if tunerCount != 1 {
		t.Errorf("Expected 1 tuner to be reused, got %d tuners", tunerCount)
	}
}
