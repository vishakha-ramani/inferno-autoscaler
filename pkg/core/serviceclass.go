package core

import (
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

type ServiceClass struct {
	Spec *config.ServiceClassSpec

	// for all models, for all accelerators
	AllAllocations map[string]map[string]*Allocation

	// allocated solution for all models
	allocation map[string]*Allocation
}

func NewServiceClassFromSpec(spec *config.ServiceClassSpec) *ServiceClass {
	return &ServiceClass{
		Spec:           spec,
		AllAllocations: make(map[string]map[string]*Allocation),
		allocation:     map[string]*Allocation{},
	}
}

func (c *ServiceClass) Calculate(models map[string]*Model, accelerators map[string]*Accelerator) {
	for _, ml := range c.Spec.ModelLoad {
		modelName := ml.Name
		model := models[modelName]
		c.AllAllocations[modelName] = make(map[string]*Allocation)
		for _, g := range accelerators {
			if alloc := CreateAllocation(model, g, &ml); alloc != nil {
				c.AllAllocations[modelName][g.Name] = alloc
			}
		}
	}
}

func (c *ServiceClass) String() string {
	return fmt.Sprintf("ServiceClass: name=%s; load=%v; allAllocations=%v; allocation=%v",
		c.Spec.Name, c.Spec.ModelLoad, c.AllAllocations, c.allocation)
}
