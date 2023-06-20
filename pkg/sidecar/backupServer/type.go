package backupServer

const (
	// xtrabackup Executable Name
	xtrabackupCommand = "xtrabackup"

	mcCommand = "mc"

	// xbstream Executable Name
	xbstreamCommand = "xbstream"

	// SidecarServerPort represents the port on which http server will run
	ServerPort = 19999

	// DataDir is the mysql data. /var/lib/mysql
	dataDir = "/greatdb/mysql/data"

	// ServerProbeEndpoint is the http server endpoint for probe
	serverProbeEndpoint = "/health"

	// ServerProbeEndpoint is the http server endpoint for upload
	serverUploadEndpoint = "/upload"

	// ServerBackupEndpoint is the http server endpoint for backups
	serverBackupEndpoint = "/xbackup"

	// serverBackupInfoEndpoint is the http server endpoint for backups
	ServerBackupInfoEndpoint = "/backupinfo"

	ServerBackupDownEndpoint = "/backupdown"

	uploadStatusTrailer = "X-Upload-Status"
	uploadSuccessful    = "Success"
	uploadFailed        = "Failed"

	backupStatusTrailer = "X-Backup-Status"
	backupReasonTrailer = "X-Backup-Reason"
	backupSuccessful    = "Success"
	backupFailed        = "Failed"

	GreatdbBackupName        = "backup.stream"
	GreatdbClusterBackupName = "cluster-backup.tar.gz"

	GreatdbBackupDataMountPath = "/backup"
	GreatdbDataMountPath       = "/greatdb/mysql"
	GreatdbRestorePath         = GreatdbDataMountPath + "/restore"
)
