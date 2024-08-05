package main

import (
	"fmt"
	"os"

	core "github.ibm.com/tantawi/inferno/pkg/core"
)

func main() {
	prefix := "../../samples/"
	fn_acc := prefix + "systemData.json"
	fn_mod := prefix + "modelData.json"
	fn_srv := prefix + "serviceClassData.json"

	system := core.NewSystem()

	bytes_acc, err_acc := os.ReadFile(fn_acc)
	if err_acc != nil {
		fmt.Println(err_acc)
	}
	system.SetAccelerators(bytes_acc)

	bytes_mod, err_mod := os.ReadFile(fn_mod)
	if err_mod != nil {
		fmt.Println(err_mod)
	}
	system.SetModels(bytes_mod)

	bytes_srv, err_srv := os.ReadFile(fn_srv)
	if err_srv != nil {
		fmt.Println(err_srv)
	}
	system.SetServiceClasses(bytes_srv)

	system.Calculate()

	solver := core.NewSolver()
	//solver.SolveUnlimited(system)
	solver.Solve(system)

	system.AllocateByType()
	fmt.Printf("%v", system)
}
