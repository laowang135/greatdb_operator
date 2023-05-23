package resources

import (
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"

	"greatdb-operator/pkg/resources/configmap"

	"greatdb-operator/pkg/resources/secret"

	"greatdb-operator/pkg/resources/service"

	"greatdb-operator/pkg/resources/greatdbpaxos"
	"greatdb-operator/pkg/resources/pods"

	"k8s.io/client-go/tools/record"
)

type GreatDBPaxosResourceManagers struct {
	ConfigMap resources.Manager
	Service   resources.Manager
	Secret    resources.Manager
	GreatDB   resources.Manager
}

func NewGreatDBPaxosResourceManagers(client *deps.ClientSet, listers *deps.Listers, recorder record.EventRecorder) *GreatDBPaxosResourceManagers {
	configmap := &configmap.ConfigMapManager{Client: client, Listers: listers, Recorder: recorder}
	service := &service.ServiceManager{Client: client, Listers: listers, Recorder: recorder}
	secret := &secret.SecretManager{Client: client, Lister: listers, Recorder: recorder}

	gdb := &greatdbpaxos.GreatDBManager{Client: client, Lister: listers, Recorder: recorder}
	return &GreatDBPaxosResourceManagers{
		ConfigMap: configmap,
		Service:   service,
		Secret:    secret,
		GreatDB:   gdb,
	}

}

type ReadAndWriteManager struct {
	Pods resources.Manager
}

func NewReadAndWriteManager(client *deps.ClientSet, listers *deps.Listers) *ReadAndWriteManager {
	pod := pods.ReadAndWriteManager{Client: client, Lister: listers}
	return &ReadAndWriteManager{
		Pods: pod,
	}
}
