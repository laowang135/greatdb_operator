package utils

var (
	ApiSupportedWebhookVersions = []string{"v1alpha1"}
)

const (
	GreatDBCreateValidatePath               = "/greatdbpaxos-validate-create"
	GreatDBUpdateValidatePath               = "/greatdbpaxos-validate-update"
	GreatDBBackupRecordCreateValidatePath   = "/greatdbbackup-validate-create"
	GreatDBBackupRecordUpdateValidatePath   = "/greatdbbackup-validate-update"
	GreatDBBackupScheduleCreateValidatePath = "/greatdbbackupschedule-validate-create"
	GreatDBBackupScheduleUpdateValidatePath = "/greatdbbackupschedule-validate-update"
)
