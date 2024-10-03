package manager

import (
	"github.ibm.com/tantawi/inferno/pkg/core"
	"github.ibm.com/tantawi/inferno/pkg/solver"
)

type Manager struct {
	system    *core.System
	optimizer *solver.Optimizer
}

func NewManager(system *core.System, optimizer *solver.Optimizer) *Manager {
	return &Manager{
		system:    system,
		optimizer: optimizer,
	}
}

func (m *Manager) Optimize() {
	m.optimizer.Optimize(m.system)
}
