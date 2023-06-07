package greatdb_api

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"k8s.io/client-go/kubernetes"

	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks"
	"greatdb-operator/pkg/utils/log"
)

func configTLS(tlsKeyFile, tlsCertFile string) (*tls.Config, error) {
	sCert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
	if err != nil {

		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
	}, nil
}

func register(kubeClient kubernetes.Interface, greatdbClient versioned.Interface) {
	// webhook
	api := webhooks.GreatDBWebhookServer{}
	api.Server(kubeClient, greatdbClient)

}

func Server(greatDBClient versioned.Interface, kubeClient kubernetes.Interface, tlsKeyFile, tlsCertFile string, port int, enableWebhooks bool, stop <-chan struct{}) {

	if enableWebhooks {
		if port == 8080 {
			panic("Port 8080 is a fixed port for health check and cannot be used")
		}
		if tlsKeyFile == "" || tlsCertFile == "" {
			panic("The parameters tlsKeyFile and tlsCertFile are required after the webhooks service is enabled")
		}
		register(kubeClient, greatDBClient)

		tlsCfg, err := configTLS(tlsKeyFile, tlsCertFile)
		if err != nil {
			log.Log.Reason(err).Error("Failed to start webhooks service")
			panic(err)
		}
		server := http.Server{
			Addr:      fmt.Sprintf(":%d", port),
			TLSConfig: tlsCfg,
		}

		log.Log.V(2).Info("Starting api server")
		go func() {
			err = server.ListenAndServeTLS("", "")
			if err != nil {
				log.Log.Errorf("api server start failed: %s", err.Error())
				panic(err)
			}
			log.Log.V(2).Info("successfully started the webhooks api service")
		}()

	}
	// health
	go healthServer(greatDBClient)

	<-stop

}
