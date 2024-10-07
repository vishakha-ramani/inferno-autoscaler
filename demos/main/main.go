package main

import (
	"fmt"
	"os"

	core "github.ibm.com/tantawi/inferno/pkg/core"
	"github.ibm.com/tantawi/inferno/pkg/manager"
	"github.ibm.com/tantawi/inferno/pkg/solver"
)

func main() {
	size := "large"
	if len(os.Args) > 1 {
		size = os.Args[1]
	}
	prefix := "../../samples/" + size + "/"
	fn_acc := prefix + "accelerator-data.json"
	fn_mod := prefix + "model-data.json"
	fn_svc := prefix + "serviceclass-data.json"
	fn_srv := prefix + "server-data.json"
	fn_opt := prefix + "optimizer-data.json"
	fn_sol := prefix + "solution-data.json"

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

	bytes_svc, err_svc := os.ReadFile(fn_svc)
	if err_svc != nil {
		fmt.Println(err_svc)
	}
	system.SetServiceClassesFromSpec(bytes_svc)

	bytes_srv, err_srv := os.ReadFile(fn_srv)
	if err_srv != nil {
		fmt.Println(err_srv)
	}
	system.SetServersFromSpec(bytes_srv)

	bytes_opt, err_opt := os.ReadFile(fn_opt)
	if err_opt != nil {
		fmt.Println(err_acc)
	}
	optimizer, err_opt := solver.NewOptimizerFromSpec(bytes_opt)
	if err_opt != nil {
		fmt.Println(err_acc)
	}

	manager := manager.NewManager(system, optimizer)

	system.Calculate()
	manager.Optimize()

	bytes_sol, err_sol := system.GetSolution()
	if err_sol != nil {
		fmt.Println(err_sol)
	} else {
		os.WriteFile(fn_sol, bytes_sol, 0644)
	}

	fmt.Printf("%v", system)
	fmt.Printf("%v", optimizer)
}
