package collector

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.ibm.com/tantawi/inferno/pkg/config"
	ctrl "github.ibm.com/tantawi/inferno/services/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Handlers for REST API calls

func collect(c *gin.Context) {
	// get managed deployments
	labelSelector := ctrl.KeyManaged + "=true"
	deps, err := KubeClient.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector})
	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "kube client: " + err.Error()})
		return
	}

	// initialize collector info
	serverSpecs := make([]config.ServerSpec, 0)
	serverMap := make(map[string]ctrl.ServerKubeInfo)

	// collect data from deployments
	for _, d := range deps.Items {

		if d.ObjectMeta.Labels == nil || d.ObjectMeta.Labels[ctrl.KeyServerName] == "" {
			continue
		}
		serverName := d.ObjectMeta.Labels[ctrl.KeyServerName]

		depUID := string(d.UID)
		serverMap[serverName] = ctrl.ServerKubeInfo{
			UID:   depUID,
			Name:  d.ObjectMeta.Name,
			Space: d.ObjectMeta.Namespace,
		}

		numReplicas := *d.Spec.Replicas
		maxBatchSize, _ := strconv.Atoi(d.ObjectMeta.Labels[ctrl.KeyMaxBatchSize])
		arrvRate, _ := strconv.ParseFloat(d.ObjectMeta.Labels[ctrl.KeyArrivalRate], 32)
		avglength, _ := strconv.Atoi(d.ObjectMeta.Labels[ctrl.KeyNumTokens])

		curAlloc := config.AllocationData{
			Accelerator: d.ObjectMeta.Labels[ctrl.KeyAccelerator],
			NumReplicas: int(numReplicas),
			MaxBatch:    maxBatchSize,
			Load: config.ServerLoadSpec{
				ArrivalRate: float32(arrvRate),
				AvgLength:   avglength,
			},
		}

		serverSpec := config.ServerSpec{
			Name:         serverName,
			Class:        d.ObjectMeta.Labels[ctrl.KeyServerClass],
			Model:        d.ObjectMeta.Labels[ctrl.KeyServerModel],
			CurrentAlloc: curAlloc,
		}
		serverSpecs = append(serverSpecs, serverSpec)
	}

	serverCollectorInfo := ctrl.ServerCollectorInfo{
		Spec:         serverSpecs,
		KubeResource: serverMap,
	}

	c.IndentedJSON(http.StatusOK, serverCollectorInfo)
}
