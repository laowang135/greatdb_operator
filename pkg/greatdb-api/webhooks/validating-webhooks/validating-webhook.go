package validating_webhooks

import (
	"net/http"

	"k8s.io/client-go/kubernetes"

	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	"greatdb-operator/pkg/greatdb-api/webhooks/validating-webhooks/admitters"
)

func ServeGreatDBPaxosCreate(resp http.ResponseWriter, req *http.Request, kubeClient kubernetes.Interface) {
	utils.Serve(resp, req, &admitters.GreatDBClusterCreateAdmitter{Client: kubeClient})
}

func ServeGreatDBPaxosUpdate(resp http.ResponseWriter, req *http.Request, kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {
	utils.Serve(resp, req, &admitters.GreatDBClusterUpdateAdmitter{KubeClient: kubeClient, GreatDBClient: greatdbClient})
}

func ServeGreatDBBackupScheduleCreate(resp http.ResponseWriter, req *http.Request, kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {
	utils.Serve(resp, req, &admitters.GreatDBBackupScheduleCreateAdmitter{KubeClient: kubeClient, GreatDBClient: greatdbClient})
}

func ServeGreatDBBackupScheduleUpdate(resp http.ResponseWriter, req *http.Request, kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {
	utils.Serve(resp, req, &admitters.GreatDBBackupScheduleUpdateAdmitter{KubeClient: kubeClient, GreatDBClient: greatdbClient})
}

func ServeGreatDBBackupRecordCreate(resp http.ResponseWriter, req *http.Request, kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {
	utils.Serve(resp, req, &admitters.GreatDBBackupRecordCreateAdmitter{KubeClient: kubeClient, GreatDBClient: greatdbClient})
}

func ServeGreatDBBackupRecordUpdate(resp http.ResponseWriter, req *http.Request, kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {
	utils.Serve(resp, req, &admitters.GreatDBBackupRecordUpdateAdmitter{KubeClient: kubeClient, GreatDBClient: greatdbClient})
}
