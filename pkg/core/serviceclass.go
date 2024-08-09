package core

import (
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// A service class
type ServiceClass struct {
	spec *config.ServiceClassSpec

	// model load data for all models
	modelLoad map[string]*config.LoadSpec

	// for all models, for all accelerators
	allAllocations map[string]map[string]*Allocation

	// allocated solution for all models
	allocation map[string]*Allocation
}

func NewServiceClassFromSpec(spec *config.ServiceClassSpec) *ServiceClass {
	modelLoad := make(map[string]*config.LoadSpec)
	for _, ml := range spec.Load {
		modelLoad[ml.Name] = &ml
	}
	return &ServiceClass{
		spec:           spec,
		modelLoad:      modelLoad,
		allAllocations: make(map[string]map[string]*Allocation),
		allocation:     map[string]*Allocation{},
	}
}

// Calculate allocations for all models on all accelerators
func (c *ServiceClass) Calculate(models map[string]*Model, accelerators map[string]*Accelerator) {
	for modelName, ml := range c.modelLoad {
		model := models[modelName]
		if model == nil {
			continue
		}
		c.allAllocations[modelName] = make(map[string]*Allocation)
		for _, g := range accelerators {
			if alloc := CreateAllocation(model, g, ml); alloc != nil {
				if curAlloc := c.allocation[modelName]; curAlloc != nil {
					penalty := curAlloc.TransitionPenalty(alloc)
					alloc.SetValue(penalty)
				}
				c.allAllocations[modelName][g.name] = alloc
			}
		}
	}
}

func (c *ServiceClass) GetName() string {
	return c.spec.Name
}

// The allocated solution for a model; could be nil
func (c *ServiceClass) GetModelAllocation(modelName string) *Allocation {
	return c.allocation[modelName]
}

// The load data for a model; could be nil
func (c *ServiceClass) GetModelLoad(modelName string) *config.LoadSpec {
	return c.modelLoad[modelName]
}

func (c *ServiceClass) String() string {
	return fmt.Sprintf("ServiceClass: name=%s; load=%v; allAllocations=%v; allocation=%v",
		c.spec.Name, c.spec.Load, c.allAllocations, c.allocation)
}
