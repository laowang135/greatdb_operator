package greatdb_api

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"greatdb-operator/pkg/client/clientset/versioned"

	"github.com/emicklei/go-restful"
	"k8s.io/apimachinery/pkg/util/json"
)

type KubeApiHealthzVersion struct {
	version interface{}
	sync.RWMutex
}

func (h *KubeApiHealthzVersion) Update(body interface{}) {
	h.Lock()
	defer h.Unlock()
	h.version = body
}

func (h *KubeApiHealthzVersion) Clear() {
	h.Lock()
	defer h.Unlock()
	h.version = nil
}

func (h *KubeApiHealthzVersion) GetVersion() (v interface{}) {
	h.RLock()
	defer h.RUnlock()
	v = h.version
	return
}

/*
   This check is primarily to determine whether a controller can reach the Kubernetes API.
   We can reflect this based on other connections we depend on (informers and their error handling),
   rather than testing the kubernetes API every time the healthcheck endpoint is called. This
   should avoid a lot of unnecessary calls to the API while informers are healthy.

   Note that It is possible for the contents of a KubeApiHealthzVersion to be out of date if the
   Kubernetes API version changes without an informer disconnect, or if informer doesn't call
   KubeApiHealthzVersion.Clear() when it encounters an error.
*/

func KubeConnectionHealthzFuncFactory(hVersion *KubeApiHealthzVersion, client versioned.Interface) func(_ *restful.Request, response *restful.Response) {
	return func(req *restful.Request, response *restful.Response) {
		res := map[string]interface{}{}
		var version = hVersion.GetVersion()

		if version == nil {
			body, err := client.GreatdbV1alpha1().RESTClient().Get().AbsPath("/version").Do(context.Background()).Raw()
			if err != nil {
				unhealthy(err, response)
				return
			}
			err = json.Unmarshal(body, &version)
			if err != nil {
				unhealthy(err, response)
				return
			}
			hVersion.Update(version)
		}

		res["apiserver"] = map[string]interface{}{"connectivity": "ok", "version": version}
		response.WriteHeaderAndJson(http.StatusOK, res, restful.MIME_JSON)
	}
}

func unhealthy(err error, response *restful.Response) {
	res := map[string]interface{}{}
	res["apiserver"] = map[string]interface{}{"connectivity": "failed", "error": fmt.Sprintf("%v", err)}
	response.WriteHeaderAndJson(http.StatusInternalServerError, res, restful.MIME_JSON)
}

func healthServer(client versioned.Interface) {
	healthVersion := KubeApiHealthzVersion{nil, sync.RWMutex{}}

	mux := restful.NewContainer()
	webService := new(restful.WebService)
	webService.Path("/").Consumes(restful.MIME_JSON).Produces(restful.MIME_JSON)
	webService.Route(webService.GET("/healthz").To(KubeConnectionHealthzFuncFactory(&healthVersion, client)).Doc("Health endpoint"))
	mux.Add(webService)

	svc := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	err := svc.ListenAndServe()
	if err != nil {
		panic(err)
	}
}
