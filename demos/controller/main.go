package main

import (
	"fmt"
	"os"
	"strconv"

	ctrl "github.ibm.com/tantawi/inferno/services/controller"
)

// create and run a Controller
func main() {
	// provide help
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Println("Args: " + " <controlPeriodInSec>")
		return
	}

	// get args
	period := ctrl.DefaultControlPeriodSeconds
	if len(os.Args) > 1 {
		if periodArg, err := strconv.Atoi(os.Args[1]); err == nil && periodArg > 0 {
			period = periodArg
		} else {
			fmt.Println("bad argument for control period " + os.Args[1])
			return
		}
	} else {
		if periodEnvStr := os.Getenv(ctrl.ControlPeriodEnvName); periodEnvStr != "" {
			if periodEnv, err := strconv.Atoi(periodEnvStr); err == nil && periodEnv > 0 {
				period = periodEnv
			} else {
				fmt.Println("bad env variable for control period " + periodEnvStr)
				return
			}
		}
	}
	fmt.Printf("Running with control period = %d sec\n", period)

	controller, err := ctrl.NewController()
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := controller.Init(); err != nil {
		fmt.Println(err)
		return
	}
	controller.Run(period)
	ctrl.Wg.Wait()
}
