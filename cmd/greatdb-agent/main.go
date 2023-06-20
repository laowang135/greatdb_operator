package main

import (
	"flag"
	"os"
	"strconv"
	"strings"

	backupServer "greatdb-operator/pkg/sidecar/backupServer"
	dblog "greatdb-operator/pkg/utils/log"
	"greatdb-operator/pkg/utils/signals"
)

var (
	address = flag.String("address", GetStrEnvDefault("SERVER_ADDRESS", "0.0.0.0"), "The server listen address")
	port    = flag.Int("port", GetIntEnvDefault("SERVER_PORT", 19999), "The server port, default: 19999")
	mode    = flag.String("mode", GetStrEnvDefault("SERVER_MODE", "server"), "The dbinit run mode {backup restore server upload download}. default server mode.")

	fileName     = flag.String("file", GetStrEnvDefault("TRAN_FILENAME", ""), "Upload or download file name.")
	uploadPath   = flag.String("uploadPath", GetStrEnvDefault("UPLOAD_PATH", ""), "uploadPath.")
	downloadPath = flag.String("downloadPath", GetStrEnvDefault("DOWNLOAD_PATH", ""), "downloadPath.")
	chunkSize    = flag.Int("chunk", GetIntEnvDefault("TRAN_CHUNK_SIZE", 4096), "Transport block chunk size")

	log = dblog.Log
)

func GetStrEnvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func GetIntEnvDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	intVar, err := strconv.Atoi(v)
	if err != nil {
		panic(err)
	}
	return intVar
}

func main() {

	flag.Parse()
	log.Infof("run mode %s", strings.ToUpper(*mode))

	switch strings.ToLower(*mode) {
	case "greatdb-backup":
		err := backupServer.RunGreatDBBackupClient()
		if err != nil {
			log.Errorf("%v, run client command failed", err)
			os.Exit(1)
		}
	case "greatdb-restore":
		err := backupServer.RunRestore()
		if err != nil {
			log.Errorf("%v, run restore command failed", err)
			os.Exit(1)
		}

	case "server":
		stopCh := signals.SetupSignalHandler()
		if err := backupServer.RunServer(stopCh); err != nil {
			panic(err)
		}

	default:
		log.Errorf("run mode error: %s. choice {greatdb-backup, greatdb-restore, server}", *mode)
	}
}
