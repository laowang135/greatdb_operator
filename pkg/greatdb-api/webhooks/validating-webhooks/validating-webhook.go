package validating_webhooks

import (
	"net/http"

	"k8s.io/client-go/kubernetes"

	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	"greatdb-operator/pkg/greatdb-api/webhooks/validating-webhooks/admitters"
)

func ServeGreatDBClusterCreate(resp http.ResponseWriter, req *http.Request, kubeClient kubernetes.Interface) {
	utils.Serve(resp, req, &admitters.GreatDBClusterCreateAdmitter{Client: kubeClient})
}

func ServeGreatDBClusterUpdate(resp http.ResponseWriter, req *http.Request, kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {
	utils.Serve(resp, req, &admitters.GreatDBClusterUpdateAdmitter{KubeClient: kubeClient, GreatDBClient: greatdbClient})
}
