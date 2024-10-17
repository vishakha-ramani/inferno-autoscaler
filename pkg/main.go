package main

import (
	"os"

	"github.ibm.com/tantawi/inferno/pkg/rest"
)

// create and run a REST API server
func main() {
	var server rest.RESTServer
	statefull := len(os.Args) > 1 && os.Args[1] == rest.DefaultStatefull
	if statefull {
		server = rest.NewStateFullServer()
	} else {
		server = rest.NewStateLessServer()
	}
	server.Run()
}
