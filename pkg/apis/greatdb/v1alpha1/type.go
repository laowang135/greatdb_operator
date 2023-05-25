package v1alpha1

import "strings"

type UpgradeStrategyType string

const (
	AllUpgrade     UpgradeStrategyType = "all"
	RollingUpgrade UpgradeStrategyType = "rollingUpgrade"
)

type PauseModeType string

const (
	ClusterPause PauseModeType = "cluster"
	InsPause     PauseModeType = "ins"
)

type RestartModeType string

const (
	ClusterRestart RestartModeType = "cluster"
	InsRestart     RestartModeType = "ins"
)

type RestartStrategyType string

const (
	AllRestart     RestartStrategyType = "all"
	RollingRestart RestartStrategyType = "rolling"
)

type ClusterStatusType string

const (
	ClusterStatusPending ClusterStatusType = "Pending"
	ClusterStatusRunning ClusterStatusType = "Running"
	ClusterStatusFailed  ClusterStatusType = "Failed"
)

type ConditionStatus string

// These are valid condition statuses. "ConditionTrue" means a resource is in the condition.
// "ConditionFalse" means a resource is not in the condition. "ConditionUnknown" means operator
// can't decide if a resource is in the condition or not. In the future
const (
	ConditionTrue  ConditionStatus = "True"
	ConditionFalse ConditionStatus = "False"
)

// service state type of GreatDBPaxos
type GreatDBPaxosConditionType string

const (

	// The Pending phase generates relevant configurations, such as initializing configmap, secret, service
	GreatDBPaxosPending GreatDBPaxosConditionType = "Pending"

	// Deploy db cluster in this phase
	GreatDBPaxosDeployDB GreatDBPaxosConditionType = "DeployDB"

	// Boot Cluster
	GreatDBPaxosBootCluster GreatDBPaxosConditionType = "BootCluster"

	// Init User
	GreatDBPaxosInitUser GreatDBPaxosConditionType = "InitUser"

	// Successful deployment of DBscale cluster in this phase
	GreatDBPaxosSucceeded GreatDBPaxosConditionType = "Succeeded"

	// The cluster is ready at this stage
	GreatDBPaxosReady GreatDBPaxosConditionType = "Ready"

	// GreatDBPaxosDeleting The cluster is Deleting at this stage
	GreatDBPaxosTerminating GreatDBPaxosConditionType = "Terminating"

	GreatDBPaxosPause GreatDBPaxosConditionType = "Pause"
	// Restart
	GreatDBPaxosRestart GreatDBPaxosConditionType = "Restart"
	// Upgrade
	GreatDBPaxosUpgrade GreatDBPaxosConditionType = "Upgrade"
	// This stage indicates cluster deployment failure
	GreatDBPaxosFailed GreatDBPaxosConditionType = "Failed"

	// This stage indicates that the correct status of the cluster cannot be obtained due to exceptions
	GreatDBPaxosUnknown GreatDBPaxosConditionType = "Unknown"
)

func (g GreatDBPaxosConditionType) Stage() int {
	switch g {
	case GreatDBPaxosPending:
		return 0
	case GreatDBPaxosDeployDB:
		return 1
	case GreatDBPaxosBootCluster:
		return 2
	case GreatDBPaxosInitUser:
		return 3
	case GreatDBPaxosSucceeded:
		return 4
	case GreatDBPaxosReady:
		return 5
	}
	return 6
}

// Status of the greatdb instance
type MemberConditionType string

const (

	// MemberStatusOnline Indicates that the member is functioning normally and is in a state that can accept read and write requests.
	MemberStatusOnline MemberConditionType = "ONLINE"

	// MemberStatusRecovering Indicates that a member is undergoing fault recovery and may be a newly added or re joined member of the group,
	// obtaining data from other members to achieve consistency.
	MemberStatusRecovering MemberConditionType = "RECOVERING"

	// MemberStatusUnreachable Indicates that a member cannot communicate with other members in the group.
	// This may be caused by network issues, host failures, or other reasons.
	MemberStatusUnreachable MemberConditionType = "UNREACHABLE"

	// MemberStatusOffline Indicates that the member has taken the initiative to go offline and no longer participates in the operation of the group.
	// Offline members will no longer receive or process read and write requests.
	MemberStatusOffline MemberConditionType = "OFFLINE"

	// MemberStatusError Indicates that a member has encountered a serious error and is unable to function properly. Members need to be repaired or reconfigured.
	MemberStatusError MemberConditionType = "ERROR"

	// MemberStatusPending The instance is still starting
	MemberStatusPending MemberConditionType = "Pending"

	// MemberStatusFree The member has not yet joined the cluster
	MemberStatusFree MemberConditionType = "Free"

	// MemberStatusPause  Member Pause
	MemberStatusPause MemberConditionType = "Pause"

	// MemberStatusRestart Member restart
	MemberStatusRestart MemberConditionType = "Restart"

	// MemberStatusFailure  The member was changed to a failed state due to a long-term error
	MemberStatusFailure MemberConditionType = "Failure"

	// MemberStatusUnknown
	MemberStatusUnknown MemberConditionType = "Unknown"
)

func (state MemberConditionType) Parse() MemberConditionType {

	return state
}

func (state MemberConditionType) IsNormal() bool {

	switch state {
	case MemberStatusOffline, MemberStatusUnreachable, MemberStatusError:
		return false
	default:
		return true
	}

}

type MemberRoleType string

const (
	MemberRolePrimary   MemberRoleType = "PRIMARY"
	MemberRoleSecondary MemberRoleType = "SECONDARY"
	MemberRoleUnknown   MemberRoleType = "Unknown"
)

func (role MemberRoleType) Parse() MemberRoleType {

	return MemberRoleType(strings.ToUpper(string(role)))

}

type MemberCreateType string

const (
	// InitCreateMember Instance created during component clustering
	InitCreateMember MemberCreateType = "init"
	// ScaleCreateMember Instance created during expansion
	ScaleCreateMember MemberCreateType = "scaleOut"

	// FailOverCreateMember Instance created during failover
	FailOverCreateMember MemberCreateType = "failover"
)
