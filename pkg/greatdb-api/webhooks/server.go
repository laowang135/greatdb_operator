package webhooks

import (
	"net/http"

	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	"greatdb-operator/pkg/greatdb-api/webhooks/validating-webhooks"

	"k8s.io/client-go/kubernetes"
)

type GreatDBWebhookServer struct {
}

func (great GreatDBWebhookServer) registerValidatingWebhooks(kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {

	http.HandleFunc(utils.GreatDBCreateValidatePath, func(w http.ResponseWriter, r *http.Request) {
		validating_webhooks.ServeGreatDBClusterCreate(w, r, kubeClient)
	})
	http.HandleFunc(utils.GreatDBUpdateValidatePath, func(w http.ResponseWriter, r *http.Request) {
		validating_webhooks.ServeGreatDBClusterUpdate(w, r, kubeClient, greatdbClient)
	})

}

func (great GreatDBWebhookServer) Server(kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {
	great.registerValidatingWebhooks(kubeClient, greatdbClient)
}
