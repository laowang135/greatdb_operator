package greatdbpaxos

import "errors"

const (
	ControllerName string = "greatdbClusterController"

	ZookeeperDefaultImages = "docker.io/pravega/zookeeper:latest"
)

var (
	handlingLimitErr = errors.New("handling speed limit")
)
