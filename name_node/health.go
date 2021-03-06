package namenode

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/SayedAlesawy/Videra-Storage/data_node/dnpb"
	"github.com/SayedAlesawy/Videra-Storage/utils/errors"
	"google.golang.org/grpc"
)

// PingDataNodes A function to ping all currently conneced data nodes for health checking
func (nameNode *NameNode) PingDataNodes() {
	for range time.Tick(nameNode.HealthCheckInterval) {
		for _, dataNode := range nameNode.GetAllDataNodeData() {
			nameNode.pingDataNode(dataNode)
		}
	}
}

// pingDataNode A function to ping a certain data node
func (nameNode *NameNode) pingDataNode(dataNode DataNodeData) {
	address := nameNode.getDataNodeInternalAddress(dataNode)

	conn, err := grpc.Dial(address, grpc.WithInsecure())
	defer conn.Close()
	if errors.IsError(err) {
		log.Println(fmt.Sprintf("%s Unable to connect to data node on: %s", logPrefix, address))
		return
	}

	client := dnpb.NewDataNodeInternalRoutesClient(conn)
	req := dnpb.HealthCheckRequest{}

	ctx, cancel := context.WithTimeout(context.Background(), nameNode.InteralReqTimeout)
	defer cancel()

	healthCheckResp, err := client.HealthCheck(ctx, &req)
	if errors.IsError(err) {
		if dataNode.Latency > nameNode.dataNodeOfflineThreshold {
			log.Println(fmt.Sprintf("%s Data node on address: %s is OFFLINE", logPrefix, address))

			nameNode.RemoveDataNodeData(dataNode)
		} else {
			dataNode.Latency++
			nameNode.InsertDataNodeData(dataNode)

			log.Println(logPrefix, fmt.Sprintf("Data node on address: %s missed a ping", address))
		}

		return
	}

	log.Println(logPrefix, fmt.Sprintf("Data node on address: %s is:", address), healthCheckResp.Status)
}
