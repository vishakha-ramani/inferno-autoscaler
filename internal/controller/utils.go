package controller

import (
	"fmt"
	"strconv"
	"time"

	llmdVariantAutoscalingV1alpha1 "github.com/llm-d-incubation/inferno-autoscaler/api/v1alpha1"
	collector "github.com/llm-d-incubation/inferno-autoscaler/internal/collector"
	"github.com/llm-d-incubation/inferno-autoscaler/internal/logger"
	infernoConfig "github.com/llm-inferno/optimizer/pkg/config"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// adapter to create inferno system data types
func createSystemData(
	acceleratorUnitCostCm map[string]string,
	serviceClassCm map[string]string,
	newInventory map[string]map[string]collector.AcceleratorModelInfo) *infernoConfig.SystemData {

	systemData := &infernoConfig.SystemData{
		Spec: infernoConfig.SystemSpec{
			Accelerators:   infernoConfig.AcceleratorData{},
			Models:         infernoConfig.ModelData{},
			ServiceClasses: infernoConfig.ServiceClassData{},
			Servers:        infernoConfig.ServerData{},
			Optimizer:      infernoConfig.OptimizerData{},
			Capacity:       infernoConfig.CapacityData{},
		},
	}

	// get accelerator data
	acceleratorData := []infernoConfig.AcceleratorSpec{}
	for key, val := range acceleratorUnitCostCm {
		cost, err := strconv.ParseFloat(val, 32)
		if err != nil {
			logger.Log.Info("failed to parse accelerator cost in configmap, skipping accelerator", "name", key)
			continue
		}
		acceleratorData = append(acceleratorData, infernoConfig.AcceleratorSpec{
			Name:         key,
			Type:         key,
			Multiplicity: 1,
			Power:        infernoConfig.PowerSpec{},
			Cost:         float32(cost),
		})
	}
	systemData.Spec.Accelerators.Spec = acceleratorData

	// get capacity data
	acceleratorMap := make(map[string]int)
	for _, nodeAccMap := range newInventory {
		for accName, info := range nodeAccMap {
			if val, exists := acceleratorMap[accName]; !exists {
				acceleratorMap[accName] = info.Count
			} else {
				acceleratorMap[accName] = val + info.Count
			}
		}
	}
	capacityData := make([]infernoConfig.AcceleratorCount, len(acceleratorMap))
	i := 0
	for key, val := range acceleratorMap {
		capacityData[i] = infernoConfig.AcceleratorCount{
			Type:  key,
			Count: val,
		}
		i++
	}
	systemData.Spec.Capacity.Count = capacityData

	// get service class data
	serviceClassData := []infernoConfig.ServiceClassSpec{}
	for key, val := range serviceClassCm {
		var sc ServiceClass
		if err := yaml.Unmarshal([]byte(val), &sc); err != nil {
			logger.Log.Info("failed to parse service class data, skipping service class", "key", key, "err", err)
			continue
		}
		for _, entry := range sc.Data {
			serviceClassSpec := infernoConfig.ServiceClassSpec{
				Name:     sc.Name,
				Priority: sc.Priority,
				Model:    entry.Model,
				SLO_ITL:  float32(entry.SLOITL),
				SLO_TTW:  float32(entry.SLOTTW),
			}
			serviceClassData = append(serviceClassData, serviceClassSpec)
		}
	}
	systemData.Spec.ServiceClasses.Spec = serviceClassData

	// set optimizer configuration
	// TODO: make it configurable
	systemData.Spec.Optimizer.Spec = infernoConfig.OptimizerSpec{
		Unlimited:     false,
		Heterogeneous: false,
		MILPSolver:    false,
		UseCplex:      false,
	}

	// initialize model data
	systemData.Spec.Models.PerfData = []infernoConfig.ModelAcceleratorPerfData{}

	// initialize dynamic server data
	systemData.Spec.Servers.Spec = []infernoConfig.ServerSpec{}

	return systemData
}

// add model accelerator pair profile data to inferno system data
func addModelAcceleratorProfileToSystemData(
	sd *infernoConfig.SystemData,
	modelName string,
	modelAcceleratorProfile *llmdVariantAutoscalingV1alpha1.AcceleratorProfile) error {

	alpha, err := strconv.ParseFloat(modelAcceleratorProfile.Alpha, 32)
	if err != nil {
		return err
	}
	beta, err := strconv.ParseFloat(modelAcceleratorProfile.Beta, 32)
	if err != nil {
		return err
	}

	sd.Spec.Models.PerfData = append(sd.Spec.Models.PerfData,
		infernoConfig.ModelAcceleratorPerfData{
			Name:         modelName,
			Acc:          modelAcceleratorProfile.Acc,
			AccCount:     modelAcceleratorProfile.AccCount,
			Alpha:        float32(alpha),
			Beta:         float32(beta),
			MaxBatchSize: modelAcceleratorProfile.MaxBatchSize,
			AtTokens:     modelAcceleratorProfile.AtTokens,
		})
	return nil
}

// add server specs to inferno system data
func addServerInfoToSystemData(
	sd *infernoConfig.SystemData,
	va *llmdVariantAutoscalingV1alpha1.VariantAutoscaling,
	className string) error {

	var arrivalRate, avgLength, cost, itlAverage, waitAverage float64
	var err error

	// server load statistics
	if arrivalRate, err = strconv.ParseFloat(va.Status.CurrentAlloc.Load.ArrivalRate, 32); err != nil {
		arrivalRate = 0
	}
	if avgLength, err = strconv.ParseFloat(va.Status.CurrentAlloc.Load.AvgLength, 32); err != nil {
		avgLength = 0
	}

	serverLoadSpec := &infernoConfig.ServerLoadSpec{
		ArrivalRate: float32(arrivalRate),
		AvgLength:   int(avgLength),
	}

	// server allocation
	if cost, err = strconv.ParseFloat(va.Status.CurrentAlloc.VariantCost, 32); err != nil {
		cost = 0
	}
	if itlAverage, err = strconv.ParseFloat(va.Status.CurrentAlloc.ITLAverage, 32); err != nil {
		itlAverage = 0
	}
	if waitAverage, err = strconv.ParseFloat(va.Status.CurrentAlloc.WaitAverage, 32); err != nil {
		waitAverage = 0
	}
	AllocationData := &infernoConfig.AllocationData{
		Accelerator: va.Status.CurrentAlloc.Accelerator,
		NumReplicas: va.Status.CurrentAlloc.NumReplicas,
		MaxBatch:    va.Status.CurrentAlloc.MaxBatch,
		Cost:        float32(cost),
		ITLAverage:  float32(itlAverage),
		WaitAverage: float32(waitAverage),
		Load:        *serverLoadSpec,
	}

	// all server data
	name := va.Name + ":" + va.Namespace
	serverSpec := &infernoConfig.ServerSpec{
		Name:         name,
		Class:        className,
		Model:        va.Spec.ModelID,
		CurrentAlloc: *AllocationData,
		DesiredAlloc: infernoConfig.AllocationData{},
	}
	sd.Spec.Servers.Spec = append(sd.Spec.Servers.Spec, *serverSpec)
	return nil
}

func setDesiredAllocation(name string,
	nameSpace string,
	allocationSolution *infernoConfig.AllocationSolution) (*llmdVariantAutoscalingV1alpha1.OptimizedAlloc, error) {
	serverName := name + ":" + nameSpace
	var allocationData infernoConfig.AllocationData
	var exists bool
	if allocationData, exists = allocationSolution.Spec[serverName]; !exists {
		return nil, fmt.Errorf("server %s not found", serverName)
	}
	optimizedAlloc := &llmdVariantAutoscalingV1alpha1.OptimizedAlloc{
		LastRunTime: metav1.NewTime(time.Now()),
		Accelerator: allocationData.Accelerator,
		NumReplicas: allocationData.NumReplicas,
	}
	return optimizedAlloc, nil
}

func findModelSLO(cmData map[string]string, targetModel string) (*ServiceClassEntry, string /* class name */, error) {
	for key, val := range cmData {
		var sc ServiceClass
		if err := yaml.Unmarshal([]byte(val), &sc); err != nil {
			return nil, "", fmt.Errorf("failed to parse %s: %w", key, err)
		}

		for _, entry := range sc.Data {
			if entry.Model == targetModel {
				return &entry, sc.Name, nil
			}
		}
	}
	return nil, "", fmt.Errorf("model %q not found in any service class", targetModel)
}

func ptr[T any](v T) *T {
	return &v
}
