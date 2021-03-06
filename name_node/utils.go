package namenode

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/SayedAlesawy/Videra-Storage/utils/errors"
)

// getDataNodeAddress A function to get a data node address
func (nameNode *NameNode) getDataNodeInternalAddress(dataNode DataNodeData) string {
	return fmt.Sprintf("%s:%s", dataNode.IP, dataNode.InternalPort)
}

// NewDataNodeData A function to obtain a new data node data object
func NewDataNodeData(id string, ip string, internalPort string, port string, gpu bool) DataNodeData {
	return DataNodeData{
		ID:              id,
		IP:              ip,
		InternalPort:    internalPort,
		Port:            port,
		GPU:             gpu,
		Latency:         0,
		RequestCount:    0,
		LastRequestTime: time.Time{},
	}
}

// encode A function to encode the data node data into json format
func (dataNodeData *DataNodeData) encode() (string, error) {
	encodedData, err := json.Marshal(dataNodeData)
	if errors.IsError(err) {
		return "", err
	}

	return string(encodedData), nil
}

// decodeDataNodeData Decodes the stringified data node data
func (nameNode *NameNode) decodeDataNodeData(encodedData string) (DataNodeData, error) {
	var dataNodeData DataNodeData

	err := json.Unmarshal([]byte(encodedData), &dataNodeData)
	if errors.IsError(err) {
		return DataNodeData{}, err
	}

	return dataNodeData, nil
}

// GetURL returns URL from ip and port
func GetURL(ip string, port string) string {
	return fmt.Sprintf("http://%s:%s", ip, port)
}
