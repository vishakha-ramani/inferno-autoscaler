/*
Copyright 2025 The llm-d Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package saturation

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/engines/executor"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/utils"
)

// NOTE: This is a placeholder for the saturation engine implementation.
// The actual logic for the saturation engine should be implemented here.

type Engine struct {
	client   client.Client
	executor executor.Executor
	// Add fields as necessary for the engine's state and configuration.
}

// NewEngine creates a new instance of the saturation engine.
func NewEngine(client client.Client) *Engine {
	engine := Engine{
		client: client,
	}

	engine.executor = executor.NewPollingExecutor(executor.PollingConfig{
		Config: executor.Config{
			OptimizeFunc: engine.optimize,
		},
		Interval:     30 * time.Second,
		RetryBackoff: 100 * time.Millisecond,
	})

	return &engine
}

// StartOptimizeLoop starts the optimization loop for the saturation engine.
// It runs until the context is cancelled.
func (e *Engine) StartOptimizeLoop(ctx context.Context) {
	e.executor.Start(ctx)
}

// optimize performs the optimization logic.
func (e *Engine) optimize(ctx context.Context) error {
	// Get all active VAs grouped by model
	activeVAs, err := utils.ActiveVariantAutoscalingByModel(ctx, e.client)
	if err != nil {
		return err
	}

	logger.Log.Debugw("Found active VariantAutoscaling resources", "count", len(activeVAs))
	// TODO: Implement optimization logic

	return nil
}
