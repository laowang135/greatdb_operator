package greatdbpaxos

type PaxosMember struct {
	ChannelName string `json:CHANNEL_NAME,omitempty`
	ID          string `json:MEMBER_ID,omitempty`
	Host        string `json:MEMBER_HOST,omitempty`
	Port        *int   `json:MEMBER_PORT,omitempty`
	State       string `json:MEMBER_STATE,omitempty`
	Role        string `json:MEMBER_ROLE,omitempty`
	Version     string `jsob:MEMBER_VERSION,omitempty`
}

// sql
const (
	// Query cluster status
	QueryClusterMemberStatus = "select * from performance_schema.replication_group_members;"
)

const (
	// greatdb
	// The name of the greatdb mount configuration
	greatdbConfigMountPath string = "/etc/greatdb/"
	greatdbDataMountPath   string = "/greatdb/mysql/"
	GreatDBContainerName          = "greatdb"
)
