package configmap

import (
	greatdbv1alpha1 "greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

type ConfigMapManager struct {
	Client   *deps.ClientSet
	Listers  *deps.Listers
	Recorder record.EventRecorder
}

func (config *ConfigMapManager) Sync(Cluster *greatdbv1alpha1.GreatDBPaxos) error {

	greatdb := NewGreatdbConfigManager(config.Client, config.Listers, config.Recorder)
	// sync configmap of greatdb
	if err := greatdb.Sync(Cluster); err != nil {
		config.Recorder.Eventf(Cluster, corev1.EventTypeNormal, SyncGreatDbConfigmapFailedReason, err.Error())
		return err
	}

	return nil

}
