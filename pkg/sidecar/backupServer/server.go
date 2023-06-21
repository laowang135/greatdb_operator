package backupServer

import (
	"context"
	"encoding/json"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"io"
	"net/http"
	"os"
	"strings"

	internalConfig "greatdb-operator/pkg/sidecar/iniconfig"
)

type server struct {
	cfg *Config
	http.Server
}

func newServer(cfg *Config, stop <-chan struct{}) *server {
	mux := http.NewServeMux()
	srv := &server{
		cfg: cfg,
		Server: http.Server{
			Addr:    fmt.Sprintf(":%d", ServerPort),
			Handler: mux,
		},
	}

	// Add handle functions
	mux.HandleFunc(serverProbeEndpoint, srv.healthHandler)
	mux.HandleFunc(serverUploadEndpoint, srv.uploadHandler)
	mux.Handle(serverBackupEndpoint, maxClients(http.HandlerFunc(srv.backupHandler), 1))
	mux.Handle(ServerBackupInfoEndpoint, http.HandlerFunc(srv.backupInfoHandler))

	fs := http.FileServer(http.Dir(GreatdbBackupDataMountPath + "/"))
	mux.Handle(ServerBackupDownEndpoint+"/", http.StripPrefix(ServerBackupDownEndpoint, fs))

	// Shutdown gracefully the http server
	go func() {
		<-stop // wait for stop signal
		if err := srv.Shutdown(context.Background()); err != nil {
			logger.Errorf("%v failed to stop http server", err)

		}
	}()

	return srv
}

// maxClients limit an http endpoint to allow just n max concurrent connections
func maxClients(h http.Handler, n int) http.Handler {
	sema := make(chan struct{}, n)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sema <- struct{}{}
		defer func() { <-sema }()

		h.ServeHTTP(w, r)
	})
}

func (s *server) uploadHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(32 << 20)

	file, header, err := r.FormFile("file")
	if err != nil {
		w.Header().Set(uploadStatusTrailer, uploadFailed)
		logger.Errorf("Cannot ParseMultipartForm file, error: %v\n", err)
		return
	}
	defer file.Close()

	filename := r.FormValue("filename")
	if filename == "" {
		w.Header().Set(uploadStatusTrailer, uploadFailed)
		logger.Errorf("Cannot ParseMultipartForm filename, error: %v\n", err)
		return
	}

	name := strings.Split(header.Filename, ".")
	fmt.Printf("File name %s\n", name[0])
	fileDir := r.FormValue("path")
	if err := os.MkdirAll(fileDir, os.ModePerm); err != nil {
		w.Header().Set(uploadStatusTrailer, uploadFailed)
		logger.Errorf("%v, failed to create backup dir", err)
		http.Error(w, "backup failed to create backup dir", http.StatusBadRequest)
		return
	}
	filepath := fileDir + "/" + filename
	out, err := os.Create(filepath)
	if err != nil {
		w.Header().Set(uploadStatusTrailer, uploadFailed)
		logger.Errorf("%v, failed to create backup file", err)
		http.Error(w, "backup failed to create backup file", http.StatusBadRequest)
		return
	}

	defer out.Close()
	_, err = io.Copy(out, file)
	if err != nil {
		w.Header().Set(uploadStatusTrailer, uploadFailed)
		logger.Errorf("%v, failed to copy backup data", err)
		http.Error(w, "backup failed to copy backup data", http.StatusBadRequest)
		return
	}
	w.Header().Set(uploadStatusTrailer, uploadSuccessful)
}

func (s *server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		logger.Errorf("%v, failed writing request", err)
	}
}

func (s *server) backupHandler(w http.ResponseWriter, r *http.Request) {

	data, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Errorf("%v, failed to read request data", err)
		http.Error(w, "backup failed", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	backupConf := BackupConfig{}
	if err := json.Unmarshal(data, &backupConf); err != nil {
		logger.Errorf("%v, failed to unmarshal backup config", err)
		http.Error(w, "backup failed", http.StatusBadRequest)
		return
	}
	err = backupProcess(s.cfg, backupConf)
	if err != nil {
		w.Header().Set(backupStatusTrailer, backupFailed)
		w.Header().Set(backupReasonTrailer, err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// success
	w.Header().Set(backupStatusTrailer, backupSuccessful)
}

func (s *server) backupInfoHandler(w http.ResponseWriter, r *http.Request) {

	data, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Errorf("%v, failed to read request data", err)
		http.Error(w, "backup failed", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	backupReq := BackupInfoRequest{}
	if err := json.Unmarshal(data, &backupReq); err != nil {
		logger.Errorf("%v, failed to unmarshal backupinfo request", err)
		http.Error(w, "get backup info failed", http.StatusBadRequest)
		return
	}
	backupRepData := []byte{}
	if backupReq.BackupResource == string(v1alpha1.GreatDBBackupResourceType) {
		file := backupReq.GetBackupInfoFile()
		if _, err := os.Stat(file); err != nil {
			logger.Errorf("%v, failed to get backup info file", file)
			http.Error(w, "get backup info failed", http.StatusBadRequest)
			return
		}
		iniParse, err := internalConfig.NewIniParserforFile(file)
		if err != nil {
			logger.Errorf("%v, failed to load backup info file", file)
			http.Error(w, "get backup info failed", http.StatusBadRequest)
			return
		}

		backupType, err1 := iniParse.GetKey("", "backup_type")
		fromLsn, err2 := iniParse.GetKey("", "from_lsn")
		toLsn, err3 := iniParse.GetKey("", "to_lsn")
		lastLsn, err4 := iniParse.GetKey("", "last_lsn")
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			logger.Errorf("%v, failed to load backup info file", file)
			http.Error(w, "get backup info failed", http.StatusBadRequest)
			return
		}
		backupRep := BackupInfoResponse{
			BackupType: string(backupType.Value()),
			FromLsn:    string(fromLsn.Value()),
			ToLsn:      string(toLsn.Value()),
			LastLsn:    string(lastLsn.Value()),
		}
		backupRepData, err = json.Marshal(backupRep)
		if err != nil {
			logger.Errorf("%v, failed to marshal backup info request", err)
			http.Error(w, "get backup info failed", http.StatusBadRequest)
			return
		}
	}
	w.Write(backupRepData)
}

func RunServer(stop <-chan struct{}) error {
	cfg := NewConfig()
	logger.Infof("start http server for backups, :%d", ServerPort)
	srv := newServer(cfg, stop)
	return srv.ListenAndServe()
}

func NewServer(cfg *Config, stop <-chan struct{}) *server {
	return newServer(cfg, stop)
}
