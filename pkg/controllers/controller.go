package controllers

import (
	greatdbInformer "greatdb-operator/pkg/client/informers/externalversions"
	deps "greatdb-operator/pkg/controllers/dependences"
	readwriteseparation "greatdb-operator/pkg/controllers/read_write_separation"

	"greatdb-operator/pkg/controllers/greatdbpaxos"

	dblog "greatdb-operator/pkg/utils/log"

	kubeinformer "k8s.io/client-go/informers"
)

type ControllerInterface interface {
	Run(threading int, stopCh <-chan struct{}) error
}

type ControllerSet map[string]*ControllerRun

type Controllers struct {
	client                   *deps.ClientSet
	listers                  *deps.Listers
	greatDBInformer          greatdbInformer.SharedInformerFactory
	kubeLabelInformerFactory kubeinformer.SharedInformerFactory
	kubeInformerFactory      kubeinformer.SharedInformerFactory
	stopCh                   <-chan struct{}
}

// ControllerRun Define the number of controller running units and running queues
type ControllerRun struct {
	worker int
	ctrl   ControllerInterface
}

func NewControllers(
	client *deps.ClientSet,
	listers *deps.Listers,
	kubeLableInformerFactory kubeinformer.SharedInformerFactory,
	kubeInformersFactory kubeinformer.SharedInformerFactory,
	greatdbInformerFactory greatdbInformer.SharedInformerFactory,
	stopCh <-chan struct{},
) *Controllers {

	return &Controllers{
		client:                   client,
		listers:                  listers,
		greatDBInformer:          greatdbInformerFactory,
		kubeLabelInformerFactory: kubeLableInformerFactory,
		kubeInformerFactory:      kubeInformersFactory,
		stopCh:                   stopCh,
	}

}

func (ctrl Controllers) Run() {
	// register controller
	ctrlSet := ctrl.register()

	// Traverse the controller set and run
	for name, contro := range ctrlSet {
		dblog.Log.Infof("Start %s controller", name)
		go contro.ctrl.Run(contro.worker, ctrl.stopCh)
	}

	dblog.Log.Info("All controllers have been started")
	<-ctrl.stopCh
}

func (ctrl Controllers) register() ControllerSet {
	greatdbClusterController := greatdbpaxos.NewGreatDBClusterController(ctrl.client, ctrl.listers, ctrl.greatDBInformer, ctrl.kubeLabelInformerFactory)

	readAndWrite := readwriteseparation.NewReadAndWriteController(ctrl.client, ctrl.listers, ctrl.kubeLabelInformerFactory)
	return ControllerSet{
		// register  greatdb Cluster Controller
		greatdbpaxos.ControllerName:        &ControllerRun{worker: 1, ctrl: greatdbClusterController},
		readwriteseparation.ControllerName: &ControllerRun{worker: 3, ctrl: readAndWrite},
	}

}
