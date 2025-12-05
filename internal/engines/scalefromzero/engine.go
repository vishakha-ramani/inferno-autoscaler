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

package scalefromzero

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/logger"
	"github.com/llm-d-incubation/workload-variant-autoscaler/internal/utils"
)

// NOTE: This is a placeholder for the scale-from-zero engine implementation.
// The actual logic for the scale-from-zero engine should be implemented here.

type Engine struct {
	client client.Client
	// Add fields as necessary for the engine's state and configuration.
}

// NewEngine creates a new instance of the scale-from-zero engine.
func NewEngine(client client.Client) *Engine {
	return &Engine{
		client: client,
		// Initialize fields as necessary.
	}
}

// StartOptimizeLoop starts the optimization loop for the scale-from-zero engine.
// It runs until the context is cancelled.
func (e *Engine) StartOptimizeLoop(ctx context.Context) {
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		for { // Infinite retry loop in case of optimization errors.
			select {
			case <-ctx.Done():
				logger.Log.Info("Context cancelled, stopping optimization loop")
				return
			default:
			}

			err := e.optimize(ctx)
			if err == nil {
				break
			}

			logger.Log.Errorf("Optimization error: %v", err)

			select {
			case <-ctx.Done():
				logger.Log.Info("Context cancelled during retry delay")
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}, 100*time.Millisecond)
}

// optimize performs the optimization logic.
func (e *Engine) optimize(ctx context.Context) error {
	// Get all inactive (replicas == 0) VAs
	inactiveVAs, err := utils.InactiveVariantAutoscalings(ctx, e.client)
	if err != nil {
		return err
	}

	logger.Log.Debugw("Found inactive VariantAutoscaling resources", "count", len(inactiveVAs))
	// TODO: Implement optimization logic

	return nil
}
