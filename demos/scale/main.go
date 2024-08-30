package main

import (
	"fmt"
	"os"

	core "github.ibm.com/tantawi/inferno/pkg/core"
)

func main() {
	size := "large"
	if len(os.Args) > 1 {
		size = os.Args[1]
	}
	prefix := "../../samples/" + size + "/"
	fn_acc := prefix + "accelerator-data.json"
	fn_mod := prefix + "model-data.json"
	fn_srv := prefix + "serviceclass-data.json"
	fn_opt := prefix + "optimizer-data.json"

	system := core.NewSystem()

	bytes_acc, err_acc := os.ReadFile(fn_acc)
	if err_acc != nil {
		fmt.Println(err_acc)
	}
	system.SetAcceleratorsFromSpec(bytes_acc)

	bytes_mod, err_mod := os.ReadFile(fn_mod)
	if err_mod != nil {
		fmt.Println(err_mod)
	}
	system.SetModelsFromSpec(bytes_mod)

	bytes_srv, err_srv := os.ReadFile(fn_srv)
	if err_srv != nil {
		fmt.Println(err_srv)
	}
	system.SetServiceClassesFromSpec(bytes_srv)

	bytes_opt, err_opt := os.ReadFile(fn_opt)
	if err_opt != nil {
		fmt.Println(err_acc)
	}
	system.SetOptimizerFromSpec(bytes_opt)

	system.Calculate()
	system.Optimize()

	className := "Premium"
	modelName := "llama3_8b"

	servClass := system.GetServiceClass(className)
	if servClass == nil {
		fmt.Printf("No service class data for class %s\n", className)
		return
	}
	model := system.GetModel(modelName)
	if model == nil {
		fmt.Printf("No model data for model %s\n", modelName)
		return
	}
	allocBefore := servClass.GetModelAllocation(modelName)
	if allocBefore == nil {
		fmt.Printf("No allocation for class %s, model %s \n", className, modelName)
		return
	}
	// change load on model
	ml := servClass.GetModelLoad(modelName)
	if ml == nil {
		fmt.Printf("No model load data for class %s, model %s \n", className, modelName)
		return
	}
	fmt.Println("AllocBefore: ", allocBefore)
	ml.ArrivalRate *= 2.5
	ml.AvgLength = int(float32(ml.AvgLength) * 1.5)

	// scale allocation
	allocAfter, inc := allocBefore.Scale(model, system.GetAccelerators(), ml)
	fmt.Println("AllocAfter: ", allocAfter)
	fmt.Println("Inc: ", inc)

	// reallocate
	var gName string
	allocAfter, gName = allocBefore.ReAllocate(model, system.GetAccelerators(), ml)
	fmt.Println("AllocAfter: ", allocAfter)
	fmt.Println("gName: ", gName)
}
