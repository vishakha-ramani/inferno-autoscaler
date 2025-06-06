package main

import (
	"fmt"

	"inferno/services/actuator"
)

// create and run an Actuator server
func main() {
	actuator, err := actuator.NewActuator()
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	actuator.Run()
}
