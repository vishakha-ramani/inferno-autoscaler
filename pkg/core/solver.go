package core

import (
	"bytes"
	"cmp"
	"fmt"
	"math"
	"slices"
)

type Solver struct {
}

func NewSolver() *Solver {
	return &Solver{}
}

type entry struct {
	sName       string        // service class
	mName       string        // model name
	curIndex    int           // current index in allocation list
	allocations []*Allocation // ordered list of allocations
	delta       float32
}

func (s *Solver) SolveUnlimited(system *System) {
	for _, v := range system.serviceClasses {
		for mName, modelMap := range v.AllAllocations {
			minCost := float32(0)
			var minAlloc *Allocation
			for _, alloc := range modelMap {
				if minCost == 0 || alloc.Cost < minCost {
					minCost = alloc.Cost
					minAlloc = alloc
				}
			}
			v.allocation[mName] = minAlloc
		}
	}
}

func (s *Solver) Solve(system *System) {

	available := make(map[string]int)
	for k := range system.capacity {
		available[k] = system.capacity[k]
	}

	var entries []*entry = make([]*entry, 0)
	for _, v := range system.serviceClasses {
		sName := v.Spec.Name
		for mName, modelMap := range v.AllAllocations {
			e := &entry{
				sName:       sName,
				mName:       mName,
				curIndex:    0,
				allocations: make([]*Allocation, len(modelMap)),
				delta:       0,
			}
			i := 0
			for _, alloc := range modelMap {
				e.allocations[i] = alloc
				i++
			}
			slices.SortFunc(e.allocations, func(a, b *Allocation) int {
				return cmp.Compare(a.Cost, b.Cost)
			})
			if len(e.allocations) > 1 {
				e.delta = e.allocations[1].Cost - e.allocations[0].Cost
			} else {
				e.delta = math.MaxFloat32
			}
			entries = append(entries, e)
		}
	}

	orderFunc := func(a, b *entry) int {
		if a.delta == b.delta {
			return cmp.Compare(b.allocations[b.curIndex].Cost, a.allocations[a.curIndex].Cost)
		}
		return cmp.Compare(b.delta, a.delta)
	}

	slices.SortFunc(entries, orderFunc)

	for len(entries) > 0 {
		top := entries[0]
		entries = entries[1:]

		if len(top.allocations) == 0 {
			continue
		}
		alloc := top.allocations[top.curIndex]
		gName := alloc.Accelerator
		replicas := alloc.NumReplicas
		acc := system.accelerators[gName]
		tName := acc.GetType()
		count := replicas * acc.Spec.Multiplicity

		if available[tName] >= count {
			available[tName] -= count
			c := system.serviceClasses[top.sName]
			c.allocation[top.mName] = alloc
		} else {
			top.curIndex++
			if top.curIndex+1 < len(top.allocations) {
				top.delta = top.allocations[top.curIndex+1].Cost - top.allocations[top.curIndex].Cost
			} else if top.curIndex == len(top.allocations) {
				continue
			} else {
				top.delta = math.MaxFloat32
			}
			i, _ := slices.BinarySearchFunc(entries, top, orderFunc)
			entries = slices.Insert(entries, i, top)
		}
	}
}

func (e *entry) String() string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "sName=%s, mName=%s, curIndex=%d, delta=%v, allocations=%v \n",
		e.sName, e.mName, e.curIndex, e.delta, e.allocations)
	return b.String()
}
