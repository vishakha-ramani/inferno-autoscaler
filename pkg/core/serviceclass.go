package core

import (
	"fmt"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// A service class
type ServiceClass struct {
	name string

	// target SLOs for each model
	targets map[string]*Target
}

// target SLOs for service class
type Target struct {
	ITL float32
	TTW float32
}

func (t *Target) String() string {
	return fmt.Sprintf("[ITL=%v, TTW=%v]",
		t.ITL, t.TTW)
}

func NewServiceClass(name string) *ServiceClass {
	return &ServiceClass{
		name:    name,
		targets: map[string]*Target{},
	}
}

// set target SLOs for a model in a service class
func (c *ServiceClass) SetTargetFromSpec(spec *config.ServiceClassSpec) {
	if spec.Name == c.name {
		c.targets[spec.Model] = &Target{
			ITL: spec.SLO_ITL,
			TTW: spec.SLO_TTW,
		}
	}
}

func (c *ServiceClass) GetName() string {
	return c.name
}

func (c *ServiceClass) GetModelTarget(modelName string) *Target {
	return c.targets[modelName]
}

func (c *ServiceClass) String() string {
	return fmt.Sprintf("ServiceClass: name=%s; targets=%v",
		c.name, c.targets)
}
