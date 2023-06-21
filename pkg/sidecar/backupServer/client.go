package backupServer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

func newGreatDBClient(cfg *Config) error {
	backupConf := BackupConfig{
		Destination:    cfg.BackupDestination,
		StorageType:    cfg.BackupStorage,
		BackupResource: cfg.BackupResource,
		IncFromLsn:     cfg.BackupIncFromLsn,
		S3: S3Config{
			Bucket:      cfg.BackupS3Bucket,
			EndpointURL: cfg.BackupS3EndpointURL,
			AccessKey:   cfg.BackupS3AccessKey,
			SecretKey:   cfg.BackupS3SecretKey,
		},
		UploadServer: UploadServerConfig{
			Address: cfg.UploadServerAddress,
			Port:    cfg.UploadServerPort,
		},
	}
	data, err := json.Marshal(backupConf)
	if err != nil {
		logger.Errorf("%v, failed to unmarshal backup config", err)
		return fmt.Errorf("backup failed")
	}

	logger.Infof("backup server url: %v", cfg.BackupServerUrl())
	logger.Infof("request data: %v", string(data))

	resp, err := http.Post(cfg.BackupServerUrl(),
		"application/json",
		bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("backup failed, http error")
	}

	if resp.Header.Get(backupStatusTrailer) != backupSuccessful {
		reason := resp.Header.Get(backupReasonTrailer)
		return fmt.Errorf("backup failed, status header not success, reason %s", reason)
	}

	logger.Info(string(body))
	return nil
}

func RunGreatDBBackupClient() error {
	cfg := NewConfig()
	logger.Info("start http client for greatdb backups")
	return newGreatDBClient(cfg)
}
