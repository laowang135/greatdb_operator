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

type PaxosMember struct {
	ChannelName string `json:"CHANNEL_NAME,omitempty"`
	ID          string `json:"MEMBER_ID,omitempty"`
	Host        string `json:"MEMBER_HOST,omitempty"`
	Port        *int   `json:"MEMBER_PORT,omitempty"`
	State       string `json:"MEMBER_STATE,omitempty"`
	Role        string `json:"MEMBER_ROLE,omitempty"`
	Version     string `json:"MEMBER_VERSION,omitempty"`
	// MEMBER_COMMUNICATION_STACK 8.0.27
	// Stack string `json:MEMBER_COMMUNICATION_STACK,omitempty`

}

// sql
const (
	// Query cluster status
	QueryClusterMemberStatus = "select CHANNEL_NAME,MEMBER_ID,MEMBER_HOST,MEMBER_PORT,MEMBER_STATE,MEMBER_ROLE,MEMBER_VERSION from performance_schema.replication_group_members;"
)

var (
	// QueryClusterMemberFields = []string{"CHANNEL_NAME", "MEMBER_ID", "MEMBER_HOST", "MEMBER_PORT", "MEMBER_STATE", "MEMBER_ROLE", "MEMBER_VERSION"}
	QueryClusterMemberFields = []string{}
)

type MembershipInfo struct {
	MemberId             string `json:member_id,omitempty`
	ViewID               string `json:view_id,omitempty`
	MemberRole           string `json:member_role,omitempty`
	MemberState          string `json:member_state,omitempty`
	MemberVersion        string `json:member_version,omitempty`
	MemberCount          int32  `json:member_count,omitempty`
	ReachableMemberCount int32  `json:reachable_member_count,omitempty`
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

	AppKubeServiceReadyLabelKey = "app.kubernetes.io/ready"
	AppKubeServiceReady         = "true"
	AppKubeServiceNotReady      = "false"
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
