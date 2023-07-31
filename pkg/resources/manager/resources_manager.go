package resources

import (
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"

	"greatdb-operator/pkg/resources/configmap"

	"greatdb-operator/pkg/resources/secret"

	"greatdb-operator/pkg/resources/service"

	"greatdb-operator/pkg/resources/greatdbbackup"
	"greatdb-operator/pkg/resources/greatdbpaxos"
	"greatdb-operator/pkg/resources/pods"

	"k8s.io/client-go/tools/record"
)

type manager interface {
	// Implementing resource synchronization logic in sync method
	Sync(*v1alpha1.GreatDBPaxos) error
}

type BackupManager interface {
	// Implementing resource synchronization logic in sync method
	Sync(*v1alpha1.GreatDBBackupSchedule) error
}

type BackupRecordManager interface {
	// Implementing resource synchronization logic in sync method
	Sync(*v1alpha1.GreatDBBackupRecord) error
}

type GreatDBPaxosResourceManagers struct {
	ConfigMap manager
	Service   manager
	Secret    manager
	GreatDB   manager
	Dashboard manager
}

func NewGreatDBPaxosResourceManagers(client *deps.ClientSet, listers *deps.Listers, recorder record.EventRecorder) *GreatDBPaxosResourceManagers {
	configmap := &configmap.ConfigMapManager{Client: client, Listers: listers, Recorder: recorder}
	service := &service.ServiceManager{Client: client, Listers: listers, Recorder: recorder}
	secret := &secret.SecretManager{Client: client, Lister: listers, Recorder: recorder}
	dashboard := &greatdbpaxos.DashboardManager{Client: client,Lister: listers,Recorder: recorder}

	gdb := &greatdbpaxos.GreatDBManager{Client: client, Lister: listers, Recorder: recorder}
	return &GreatDBPaxosResourceManagers{
		ConfigMap: configmap,
		Service:   service,
		Secret:    secret,
		GreatDB:   gdb,
		Dashboard: dashboard,
	}

}

type ReadAndWriteManager struct {
	Pods manager
}

func NewReadAndWriteManager(client *deps.ClientSet, listers *deps.Listers) *ReadAndWriteManager {
	pod := pods.ReadAndWriteManager{Client: client, Lister: listers}
	return &ReadAndWriteManager{
		Pods: pod,
	}
}

type GreatDBBackupScheduleResourceManager struct {
	Scheduler BackupManager
}

func NewGreatDBBackupScheduleResourceManager(client *deps.ClientSet, listers *deps.Listers, recorder record.EventRecorder) *GreatDBBackupScheduleResourceManager {

	scheduler := &greatdbbackup.GreatDBBackupScheduleManager{Client: client, Listers: listers, Recorder: recorder}
	return &GreatDBBackupScheduleResourceManager{
		Scheduler: scheduler,
	}

}

type GreatDBBackupRecordResourceManager struct {
	Record BackupRecordManager
}

func NewGreatDBBackupRecordResourceManager(client *deps.ClientSet, listers *deps.Listers, recorder record.EventRecorder) *GreatDBBackupRecordResourceManager {

	record := &greatdbbackup.GreatDBBackupRecordManager{Client: client, Lister: listers, Recorder: recorder}
	return &GreatDBBackupRecordResourceManager{
		Record: record,
	}

}
