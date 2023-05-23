package greatdbpaxos

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
