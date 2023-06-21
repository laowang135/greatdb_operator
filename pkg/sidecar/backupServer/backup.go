package backupServer

import (
	"bytes"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
)

func backupProcess(cfg *Config, backupConf BackupConfig) error {
	switch backupConf.BackupResource {
	case v1alpha1.GreatDBBackupResourceType:
		return GreatDBBackupProcess(cfg, backupConf)
	default:
		return fmt.Errorf("backup resource not support")
	}
}

func GreatDBBackupProcess(cfg *Config, backupConf BackupConfig) error {
	switch backupConf.StorageType {
	case v1alpha1.BackupStorageNFS:
		return GreatDBBackupFilesystem(cfg, backupConf)
	case v1alpha1.BackupStorageUploadServer:
		return GreatDBBackupUploadServer(cfg, backupConf)
	case v1alpha1.BackupStorageS3:
		return GreatDBBackupS3(cfg, backupConf)
	default:
		return fmt.Errorf("storage not support")
	}
}

func GreatDBBackupFilesystem(cfg *Config, backupConf BackupConfig) error {
	cfg.XtrabackupTargetDir = backupConf.GetBackupDir(cfg.Namespace)
	cfg.BackupIncFromLsn = backupConf.IncFromLsn

	if err := os.MkdirAll(cfg.XtrabackupTargetDir, os.ModePerm); err != nil {
		err = fmt.Errorf("failed to create backup dir, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	xtrabackup := exec.Command(xtrabackupCommand, cfg.XtrabackupArgs()...)

	outFile, err := os.OpenFile(cfg.GetBackupFile(), os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0755)
	if err != nil {
		err = fmt.Errorf("failed to open backup log file, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	defer outFile.Close()
	xtrabackup.Stdout = outFile

	errLog, err := os.OpenFile(cfg.GetBackupLogFile(), os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0755)
	if err != nil {
		err = fmt.Errorf("failed to open backup err log file, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	defer errLog.Close()
	xtrabackup.Stderr = errLog

	if err := xtrabackup.Start(); err != nil {
		err = fmt.Errorf("failed to start xtrabackup command, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	if err := xtrabackup.Wait(); err != nil {
		err = fmt.Errorf("failed waiting for xtrabackup to finish, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	return nil
}

func GreatDBBackupUploadServer(cfg *Config, backupConf BackupConfig) error {
	cfg.XtrabackupTargetDir = backupConf.GetBackupDir(cfg.Namespace)
	cfg.BackupIncFromLsn = backupConf.IncFromLsn

	if err := os.MkdirAll(cfg.XtrabackupTargetDir, os.ModePerm); err != nil {
		err = fmt.Errorf("failed to create backup dir, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	xtrabackup := exec.Command(xtrabackupCommand, cfg.XtrabackupArgs()...)

	xtrabackup.Stderr = os.Stderr
	stdout, err := xtrabackup.StdoutPipe()
	if err != nil {
		err = fmt.Errorf("failed to create stdout pipe, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	defer func() {
		// don't care
		_ = stdout.Close()

	}()
	if err := xtrabackup.Start(); err != nil {
		err = fmt.Errorf("failed to start xtrabackup command, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	fileDir := cfg.GetBackupDir()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", GreatdbBackupName)
	if err != nil {
		err = fmt.Errorf("failed to xtrabackup upload, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	if _, err = io.Copy(part, stdout); err != nil {
		err = fmt.Errorf("failed to xtrabackup xtrabackup backup, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	if err = writer.WriteField("path", fileDir); err != nil {
		err = fmt.Errorf("failed to xtrabackup backup write path, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	if err = writer.WriteField("filename", GreatdbBackupName); err != nil {
		err = fmt.Errorf("failed to xtrabackup backup write filename, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	if err = writer.Close(); err != nil {
		err = fmt.Errorf("failed to close upload multipart write, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	url := fmt.Sprintf("http://%s:%s%s", backupConf.UploadServer.Address, backupConf.UploadServer.Port, serverUploadEndpoint)

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		err = fmt.Errorf("failed to prepare request post backup, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		err = fmt.Errorf("failed to do request post backup, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	if err := xtrabackup.Wait(); err != nil {
		err = fmt.Errorf("failed to waiting for xtrabackup finish, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	if resp.Header.Get(uploadStatusTrailer) != uploadSuccessful {
		err = fmt.Errorf("failed to upload for xtrabackup finish, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	return nil
}

func GreatDBBackupS3(cfg *Config, backupConf BackupConfig) error {
	cfg.XtrabackupTargetDir = backupConf.GetBackupDir(cfg.Namespace)
	cfg.BackupIncFromLsn = backupConf.IncFromLsn

	cfg.BackupS3Bucket = backupConf.S3.Bucket
	cfg.BackupS3EndpointURL = backupConf.S3.EndpointURL
	cfg.BackupS3AccessKey = backupConf.S3.AccessKey
	cfg.BackupS3SecretKey = backupConf.S3.SecretKey

	err := GreatDBBackupFilesystem(cfg, backupConf)
	if err != nil {
		return err
	}
	backupFile := cfg.GetBackupFile()

	// mc config host add xx http://xx xx xx --api s3v4
	mcAddHost := exec.Command(mcCommand, cfg.MCAddHostArgs()...)

	err = mcAddHost.Run()
	if err != nil {
		err = fmt.Errorf("failed to mc add host, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	//  Check if the bucket is present
	mcBucketCheck := exec.Command(mcCommand, "ls", cfg.MCBucket())

	var stdout, stderr bytes.Buffer
	mcBucketCheck.Stdout = &stdout
	mcBucketCheck.Stderr = &stderr

	err = mcBucketCheck.Run()
	if err != nil {
		err = fmt.Errorf("failed to ls mc bucket, %s, %s", err.Error(), stderr.String())
		logger.Error(stdout.String())
		logger.Error(stderr.String())
		logger.Error(err.Error())
		mcBucketCreate := exec.Command(mcCommand, "mb", cfg.MCBucket())

		stdout.Reset()
		stderr.Reset()

		mcBucketCreate.Stdout = &stdout
		mcBucketCreate.Stderr = &stderr
		err = mcBucketCreate.Run()
		if err != nil {
			err = fmt.Errorf("failed to create mc bucket, %s, %s ", err.Error(), stderr.String())
			logger.Error(stdout.String())
			logger.Error(stderr.String())
			logger.Error(err.Error())
			return err
		}
	}

	// Upload backup files
	mcCopy := exec.Command(mcCommand, "cp", backupFile, cfg.MCAddress())

	logger.Info(mcCopy.String())

	mcLog, err := os.OpenFile(cfg.GetMCLog(), os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0755)
	if err != nil {
		err = fmt.Errorf("failed to open mc log file, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	defer mcLog.Close()
	mcCopy.Stdout = mcLog

	mcErrLog, err := os.OpenFile(cfg.GetMCErrorLog(), os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0755)
	if err != nil {
		err = fmt.Errorf("failed to open mc err log file, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	defer mcErrLog.Close()
	mcCopy.Stderr = mcErrLog

	if err := mcCopy.Run(); err != nil {
		err = fmt.Errorf("failed to mc cp file, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	if err := os.RemoveAll(backupFile); err != nil {
		err = fmt.Errorf("failed to delete local backup file, %s", err.Error())
		logger.Error(err.Error())
		return err
	}
	return nil
}
