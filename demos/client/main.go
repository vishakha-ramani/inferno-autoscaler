package main

import (
	"fmt"
	"time"

	"github.ibm.com/tantawi/inferno/pkg/client"
)

var TotalTime time.Duration = 60 * time.Second

func main() {
	agent := client.NewController()
	if err := agent.Init(); err != nil {
		fmt.Println(err)
		return
	}
	agent.Run()
	time.Sleep(time.Duration(TotalTime))
}
