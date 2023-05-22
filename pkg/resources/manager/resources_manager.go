package resources

import (
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"

	"greatdb-operator/pkg/resources/configmap"

	"greatdb-operator/pkg/resources/secret"

	"greatdb-operator/pkg/resources/service"

	"greatdb-operator/pkg/resources/greatdbpaxos"

	"k8s.io/client-go/tools/record"
)

type ResourceManagers struct {
	ConfigMap resources.Manager
	Service   resources.Manager
	Secret    resources.Manager
	GreatDB   resources.Manager
}

func NewResourceManagers(client *deps.ClientSet, listers *deps.Listers, recorder record.EventRecorder) *ResourceManagers {
	configmap := &configmap.ConfigMapManager{Client: client, Listers: listers, Recorder: recorder}
	service := &service.ServiceManager{Client: client, Listers: listers, Recorder: recorder}
	secret := &secret.SecretManager{Client: client, Lister: listers, Recorder: recorder}

	gdb := &greatdbpaxos.GreatDBManager{Client: client, Lister: listers, Recorder: recorder}
	return &ResourceManagers{
		ConfigMap: configmap,
		Service:   service,
		Secret:    secret,
		GreatDB:   gdb,
	}

}
