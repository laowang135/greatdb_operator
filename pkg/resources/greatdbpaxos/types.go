package greatdbpaxos

const (
	// greatdb
	// The name of the greatdb mount configuration
	greatdbConfigMountPath string = "/etc/greatdb/"
	greatdbDataMountPath   string = "/greatdb/mysql/"
	GreatDBContainerName          = "greatdb"
)

// pvc
const (
	StorageVerticalShrinkagegprohibit  = "Storage is prohibited from shrinking and configuration is rolled back"
	StorageVerticalExpansionNotSupport = "Storage does not support dynamic expansion"
)
