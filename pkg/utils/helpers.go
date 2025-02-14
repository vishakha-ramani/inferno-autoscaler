package utils

import "encoding/json"

// unmarshal a byte array to its corresponding object
func FromDataToSpec[T interface{}](byteValue []byte, t T) (*T, error) {
	var d T
	if err := json.Unmarshal(byteValue, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
