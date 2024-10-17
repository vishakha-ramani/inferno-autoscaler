package utils

import "encoding/json"

func FromDataToSpec[T interface{}](byteValue []byte, t T) (*T, error) {
	var d T
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
