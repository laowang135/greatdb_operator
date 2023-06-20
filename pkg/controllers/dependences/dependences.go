package dependences

import (
	greatdblister "greatdb-operator/pkg/client/listers/greatdb/v1alpha1"

	batchlisterv1 "k8s.io/client-go/listers/batch/v1"
	corelisterv1 "k8s.io/client-go/listers/core/v1"

	"greatdb-operator/pkg/client/clientset/versioned"

	greatdbinformers "greatdb-operator/pkg/client/informers/externalversions"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
)

type Listers struct {
	PodLister             corelisterv1.PodLister
	ConfigMapLister       corelisterv1.ConfigMapLister
	SercetLister          corelisterv1.SecretLister
	ServiceLister         corelisterv1.ServiceLister
	PvLister              corelisterv1.PersistentVolumeLister
	PvcLister             corelisterv1.PersistentVolumeClaimLister
	JobLister             batchlisterv1.JobLister
	PaxosLister           greatdblister.GreatDBPaxosLister
	BackupSchedulerLister greatdblister.GreatDBBackupScheduleLister
	BackupRecordLister    greatdblister.GreatDBBackupRecordLister
}

type ClientSet struct {
	Clientset     versioned.Interface
	KubeClientset kubernetes.Interface
}

func NewListers(
	kubeLabelInformerFactory, kubeInformerFactory kubeinformers.SharedInformerFactory,
	greatdbInformerFactory greatdbinformers.SharedInformerFactory) *Listers {

	return &Listers{

		PodLister:             kubeLabelInformerFactory.Core().V1().Pods().Lister(),
		ConfigMapLister:       kubeLabelInformerFactory.Core().V1().ConfigMaps().Lister(),
		SercetLister:          kubeLabelInformerFactory.Core().V1().Secrets().Lister(),
		ServiceLister:         kubeLabelInformerFactory.Core().V1().Services().Lister(),
		PvLister:              kubeLabelInformerFactory.Core().V1().PersistentVolumes().Lister(),
		PvcLister:             kubeLabelInformerFactory.Core().V1().PersistentVolumeClaims().Lister(),
		JobLister:             kubeInformerFactory.Batch().V1().Jobs().Lister(),
		PaxosLister:           greatdbInformerFactory.Greatdb().V1alpha1().GreatDBPaxoses().Lister(),
		BackupSchedulerLister: greatdbInformerFactory.Greatdb().V1alpha1().GreatDBBackupSchedules().Lister(),
		BackupRecordLister:    greatdbInformerFactory.Greatdb().V1alpha1().GreatDBBackupRecords().Lister(),
	}
}

func NewClientSet(
	kubeclientSet kubernetes.Interface,
	greatDBClientSet versioned.Interface,
) *ClientSet {

	return &ClientSet{
		Clientset:     greatDBClientSet,
		KubeClientset: kubeclientSet,
	}
}
