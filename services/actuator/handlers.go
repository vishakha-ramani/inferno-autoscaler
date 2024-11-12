package actuator

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	ctrl "github.ibm.com/tantawi/inferno/services/controller"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Handlers for REST API calls

func update(c *gin.Context) {
	var info ctrl.ServerActuatorInfo
	if err := c.BindJSON(&info); err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "binding error: " + err.Error()})
		return
	}
	allocMap := info.Spec
	serverMap := info.KubeResource

	// get managed deployments
	labelSelector := ctrl.KeyManaged + "=true"
	deps, err := KubeClient.AppsV1().Deployments("").List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector})
	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "kube client: " + err.Error()})
		return
	}

	// update deployments
	for serverName, allocData := range allocMap {

		var dep ctrl.ServerKubeInfo
		var exists bool
		if dep, exists = serverMap[serverName]; !exists {
			continue
		}

		acceleratorName := allocData.Accelerator
		numReplicas := int32(allocData.NumReplicas)
		maxBatchSize := allocData.MaxBatch

		deployUID := dep.UID
		deployName := dep.Name
		nameSpace := dep.Space

		// TODO: need more efficient search
		// find deployment by name
		for _, d := range deps.Items {
			if string(d.UID) == deployUID {

				// patch numReplicas and labels
				patchAcc := fmt.Sprintf(`{"op": "replace", "path": "/metadata/labels/%s", "value": "%s"}`, ctrl.KeyAccelerator, acceleratorName)
				patchBatch := fmt.Sprintf(`{"op": "replace", "path": "/metadata/labels/%s", "value": "%d"}`, ctrl.KeyMaxBatchSize, maxBatchSize)
				patchRep := fmt.Sprintf(`{"op": "replace", "path": "/spec/replicas", "value": %d}`, numReplicas)
				patchAll := []byte(`[` + patchAcc + `,` + patchBatch + `,` + patchRep + `]`)

				// TODO: fix this
				// print change - for testing
				curMaxBatchSize, _ := strconv.Atoi(d.Labels[ctrl.KeyMaxBatchSize])
				curRPM, _ := strconv.ParseFloat(d.Labels[ctrl.KeyArrivalRate], 32)
				curNumTokens, _ := strconv.Atoi(d.Labels[ctrl.KeyNumTokens])
				fmt.Printf("srv=[%s/%s/%s]: rpm=%.2f; tok=%d; acc=%s->%s; num=%d->%d; batch=%d->%d \n",
					serverName, d.Labels[ctrl.KeyServerClass], d.Labels[ctrl.KeyServerModel],
					curRPM, curNumTokens,
					d.Labels[ctrl.KeyAccelerator], acceleratorName,
					*d.Spec.Replicas, numReplicas, curMaxBatchSize, maxBatchSize)

				// update deployment
				if _, err := KubeClient.AppsV1().Deployments(nameSpace).Patch(context.Background(), deployName,
					types.JSONPatchType, patchAll, metav1.PatchOptions{}); err != nil {

					c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "kube client: " + err.Error()})
					return
				}
			}
		}
	}

	c.IndentedJSON(http.StatusOK, "Done")
}
