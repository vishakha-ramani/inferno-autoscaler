package rest

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.ibm.com/tantawi/inferno/pkg/core"
)

// global pointer to system
var system *core.System

// Base REST server
type BaseServer struct {
	router *gin.Engine
}

func NewBaseServer() *BaseServer {
	return &BaseServer{
		router: gin.Default(),
	}
}

// start server
func (server *BaseServer) Run() {
	// instantiate a clean system
	system = core.NewSystem()

	var host, port string
	if host = os.Getenv(RestHostEnvName); host == "" {
		host = DefaultRestHost
	}
	if port = os.Getenv(RestPortEnvName); port == "" {
		port = DefaultRestPort
	}
	server.router.Run(host + ":" + port)
}
