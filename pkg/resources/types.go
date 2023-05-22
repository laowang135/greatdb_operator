package resources

import (
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/config"
)

// Resource synchronization logical interface
type Manager interface {
	// Implementing resource synchronization logic in sync method
	Sync(*v1alpha1.GreatDBPaxos) error
}

// label
const (
	AppKubeNameLabelKey = "app.kubernetes.io/name"

	AppKubeComponentLabelKey = "app.kubernetes.io/component"
	AppKubeComponentGreatDB  = "GreatDB"

	AppkubeManagedByLabelKey = "app.kubernetes.io/managed-by"

	AppKubeInstanceLabelKey = "app.kubernetes.io/instance"
	AppKubePodLabelKey      = "app.kubernetes.io/pod-name"

	AppKubeGreatDBRoleLabelKey = "app.kubernetes.io/role"
)

// Finalizers
const (
	FinalizersGreatDBCluster = "greatdb.com/resources-protection"
	DefaultClusterDomain     = "cluster.local"
)

// Component suffix. The created component sts will add a suffix to the cluster name
const (
	ComponentGreatDBSuffix   = "-greatdb"
	ComponentDashboardSuffix = "-dashboard"
)

// service
const (
	ServiceRead  = "-read"
	ServiceWrite = "-write"
)

// pvc
const (
	GreatdbPvcDataName   = "data"
	GreatDBPvcConfigName = "config"
)

// user
const (
	RootPasswordKey          = "ROOTPASSWORD"
	RootPasswordDefaultValue = "greatdb@root"

	ClusterUserKey          = "ClusterUser"
	ClusterUserDefaultValue = "greatdb"
	ClusterUserPasswordKey  = "ClusterUserPassword"
)

// port

const (
	GroupPort = 33061
)

var (
	AppkubeManagedByLabelValue = "greatdb-operator"
	AppKubeNameLabelValue      = "GreatDBPaxos"
)

func init() {
	if config.ManagerBy != "" {
		AppkubeManagedByLabelValue = config.ManagerBy
	}

	if config.ServiceType != "" {
		AppKubeNameLabelValue = config.ServiceType
	}

}
