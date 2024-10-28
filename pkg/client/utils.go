package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.ibm.com/tantawi/inferno/pkg/config"
)

// get optimizer solution by sending POST to REST server
func POSTOptimize(systemData *config.SystemData) (*config.AllocationSolution, error) {
	endPoint := OptimizerURL + "/" + OptimizeVerb
	if byteValue, err := json.Marshal(systemData); err != nil {
		return nil, err
	} else {
		req, getErr := http.NewRequest("POST", endPoint, bytes.NewBuffer(byteValue))
		if getErr != nil {
			return nil, getErr
		}
		req.Header.Add("Content-Type", "application/json")
		client := &http.Client{}
		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("optimize failed to find solution: " + res.Status)
		}
		solution := config.AllocationSolution{}
		derr := json.NewDecoder(res.Body).Decode(&solution)
		if derr != nil {
			return nil, derr
		}
		return &solution, nil
	}
}

// get servers data from REST server
func GetServerData() (*config.ServerData, error) {
	endPoint := OptimizerURL + "/" + ServersVerb
	resp, getErr := http.Get(endPoint)
	if getErr != nil {
		return nil, getErr
	}
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	servers := config.ServerData{}
	jsonErr := json.Unmarshal(body, &servers)
	if jsonErr != nil {
		return nil, jsonErr
	}
	return &servers, nil
}
