package rest

// A statefull REST server with many GET and one POST API calls
type StateLessServer struct {
	BaseServer
}

// create a stateless REST server
func NewStateLessServer() *StateLessServer {
	server := &StateLessServer{
		BaseServer: *NewBaseServer(),
	}

	server.router.POST("/optimizeOne", optimizeOne)

	server.router.GET("/getAccelerators", getAccelerators)
	server.router.GET("/getAccelerator/:name", getAccelerator)

	server.router.GET("/getCapacities", getCapacities)
	server.router.GET("/getCapacity/:type", getCapacity)

	server.router.GET("/getModels", getModels)
	server.router.GET("/getModel/:name", getModel)

	server.router.GET("/getServiceClasses", getServiceClasses)
	server.router.GET("/getServiceClass/:name", getServiceClass)

	server.router.GET("/getServiceClassModelTarget/:name/:model", getServiceClassModelTarget)

	server.router.GET("/getServers", getServers)
	server.router.GET("/getServer/:name", getServer)

	server.router.GET("/getModelAcceleratorPerf/:name/:acc", getModelAcceleratorPerf)

	return server
}
