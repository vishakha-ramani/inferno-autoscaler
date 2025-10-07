package solver

import (
	"testing"

	"github.com/llm-d-incubation/workload-variant-autoscaler/hack/inferno/pkg/config"
	"github.com/llm-d-incubation/workload-variant-autoscaler/hack/inferno/pkg/core"
)

func TestNewSolver(t *testing.T) {
	tests := []struct {
		name          string
		optimizerSpec *config.OptimizerSpec
		wantErr       bool
	}{
		{
			name: "valid optimizer spec",
			optimizerSpec: &config.OptimizerSpec{
				Unlimited:        false,
				SaturationPolicy: "None",
			},
			wantErr: false,
		},
		{
			name: "unlimited optimizer spec",
			optimizerSpec: &config.OptimizerSpec{
				Unlimited:        true,
				SaturationPolicy: "PriorityExhaustive",
			},
			wantErr: false,
		},
		{
			name:          "nil optimizer spec",
			optimizerSpec: nil,
			wantErr:       false, // Constructor should handle nil gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			solver := NewSolver(tt.optimizerSpec)
			if solver == nil && !tt.wantErr {
				t.Fatal("NewSolver() returned nil unexpectedly")
			}
			if solver != nil && tt.wantErr {
				t.Fatal("NewSolver() should have failed but didn't")
			}
			if solver != nil {
				// Check that internal maps are initialized
				if solver.currentAllocation == nil {
					t.Error("currentAllocation map not initialized")
				}
				if solver.diffAllocation == nil {
					t.Error("diffAllocation map not initialized")
				}
			}
		})
	}
}

func TestSolver_Solve(t *testing.T) {
	tests := []struct {
		name          string
		optimizerSpec *config.OptimizerSpec
		setup         func(optimizerSpec *config.OptimizerSpec)
		wantErr       bool
	}{
		{
			name: "solve with limited resources - basic test",
			optimizerSpec: &config.OptimizerSpec{
				Unlimited:        false,
				SaturationPolicy: "None",
			},
			setup: func(optimizerSpec *config.OptimizerSpec) {
				system := core.NewSystem()
				system.SetFromSpec(&config.SystemSpec{
					Accelerators: config.AcceleratorData{
						Spec: []config.AcceleratorSpec{
							{
								Name: "A100",
								Power: config.PowerSpec{
									Idle:     50,
									MidPower: 150,
									Full:     350,
									MidUtil:  0.4,
								},
							},
						},
					},
					Models: config.ModelData{
						PerfData: []config.ModelAcceleratorPerfData{
							{
								Name:     "llama-7b",
								Acc:      "A100",
								AccCount: 1,
							},
						},
					},
					Capacity: config.CapacityData{
						Count: []config.AcceleratorCount{
							{
								Type:  "A100",
								Count: 2,
							},
						},
					},
					Servers: config.ServerData{
						Spec: []config.ServerSpec{
							{
								Name:            "server1",
								Class:           "default",
								Model:           "llama-7b",
								KeepAccelerator: true,
								MinNumReplicas:  1,
								MaxBatchSize:    512,
								CurrentAlloc: config.AllocationData{
									Accelerator: "A100",
									NumReplicas: 1,
								},
							},
						},
					},
					ServiceClasses: config.ServiceClassData{
						Spec: []config.ServiceClassSpec{
							{
								Name:     "default",
								Priority: 1,
								ModelTargets: []config.ModelTarget{
									{
										Model:    "llama-7b",
										SLO_ITL:  9,
										SLO_TTFT: 1000,
									},
								},
							},
						},
					},
					Optimizer: config.OptimizerData{
						Spec: *optimizerSpec,
					},
				})
				core.TheSystem = system
			},
			wantErr: false,
		},
		{
			name: "solve with unlimited resources - basic test",
			optimizerSpec: &config.OptimizerSpec{
				Unlimited:        true,
				SaturationPolicy: "None",
			},
			setup: func(optimizerSpec *config.OptimizerSpec) {
				system := core.NewSystem()
				system.SetFromSpec(&config.SystemSpec{
					Accelerators: config.AcceleratorData{
						Spec: []config.AcceleratorSpec{
							{
								Name: "A100",
								Power: config.PowerSpec{
									Idle:     50,
									MidPower: 150,
									Full:     350,
									MidUtil:  0.4,
								},
							},
						},
					},
					Models: config.ModelData{
						PerfData: []config.ModelAcceleratorPerfData{
							{
								Name:     "llama-7b",
								Acc:      "A100",
								AccCount: 1,
							},
						},
					},
					Capacity: config.CapacityData{
						Count: []config.AcceleratorCount{
							{
								Type:  "A100",
								Count: 2,
							},
						},
					},
					Servers: config.ServerData{
						Spec: []config.ServerSpec{
							{
								Name:            "server1",
								Class:           "default",
								Model:           "llama-7b",
								KeepAccelerator: true,
								MinNumReplicas:  1,
								MaxBatchSize:    512,
								CurrentAlloc: config.AllocationData{
									Accelerator: "A100",
									NumReplicas: 1,
								},
							},
						},
					},
					ServiceClasses: config.ServiceClassData{
						Spec: []config.ServiceClassSpec{
							{
								Name:     "default",
								Priority: 1,
								ModelTargets: []config.ModelTarget{
									{
										Model:    "llama-7b",
										SLO_ITL:  9,
										SLO_TTFT: 1000,
									},
								},
							},
						},
					},
					Optimizer: config.OptimizerData{
						Spec: *optimizerSpec,
					},
				})
				core.TheSystem = system
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup(tt.optimizerSpec)
			}

			solver := NewSolver(tt.optimizerSpec)
			err := solver.Solve()
			if (err != nil) != tt.wantErr {
				t.Errorf("Solver.Solve() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSolver_String(t *testing.T) {
	optimizerSpec := &config.OptimizerSpec{
		Unlimited:        false,
		SaturationPolicy: "None",
	}

	solver := NewSolver(optimizerSpec)

	// String method should not panic and return something
	str := solver.String()
	if str == "" {
		t.Error("Solver.String() returned empty string")
	}
}
