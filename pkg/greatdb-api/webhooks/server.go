package webhooks

import (
	"net/http"

	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	validating_webhooks "greatdb-operator/pkg/greatdb-api/webhooks/validating-webhooks"

	"k8s.io/client-go/kubernetes"
)

type GreatDBWebhookServer struct {
}

func (great GreatDBWebhookServer) registerValidatingWebhooks(kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {

	// greatdbpaxos
	http.HandleFunc(utils.GreatDBCreateValidatePath, func(w http.ResponseWriter, r *http.Request) {
		validating_webhooks.ServeGreatDBPaxosCreate(w, r, kubeClient, greatdbClient)
	})
	http.HandleFunc(utils.GreatDBUpdateValidatePath, func(w http.ResponseWriter, r *http.Request) {
		validating_webhooks.ServeGreatDBPaxosUpdate(w, r, kubeClient, greatdbClient)
	})

	// greatdbbackupschedule
	http.HandleFunc(utils.GreatDBBackupScheduleCreateValidatePath, func(w http.ResponseWriter, r *http.Request) {
		validating_webhooks.ServeGreatDBBackupScheduleCreate(w, r, kubeClient, greatdbClient)
	})
	http.HandleFunc(utils.GreatDBBackupScheduleUpdateValidatePath, func(w http.ResponseWriter, r *http.Request) {
		validating_webhooks.ServeGreatDBBackupScheduleUpdate(w, r, kubeClient, greatdbClient)
	})

	// greatdbbackuprecord
	http.HandleFunc(utils.GreatDBBackupRecordCreateValidatePath, func(w http.ResponseWriter, r *http.Request) {
		validating_webhooks.ServeGreatDBBackupRecordCreate(w, r, kubeClient, greatdbClient)
	})
	http.HandleFunc(utils.GreatDBBackupRecordUpdateValidatePath, func(w http.ResponseWriter, r *http.Request) {
		validating_webhooks.ServeGreatDBBackupRecordUpdate(w, r, kubeClient, greatdbClient)
	})

}

func (great GreatDBWebhookServer) Server(kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {
	great.registerValidatingWebhooks(kubeClient, greatdbClient)
}
