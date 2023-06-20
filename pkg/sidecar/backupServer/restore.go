package backupServer

import (
	"bytes"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"os"
	"os/exec"
	"strings"
)

func removeDir(path string) error {
	err := os.RemoveAll(path)
	if err != nil {
		logger.Errorf("%v, remove dir %v err", err, path)
		return fmt.Errorf("remove dir %v err", path)
	}
	logger.Infof("remove dir %v sucess", path)
	return nil
}

func downBackupFilesystem(cfg *Config, local bool) error {

	for _, path := range cfg.BackupRecordFiles {
		file := GetBackupFilePath(path)
		downDir := GetBackupFileDownloadDir(path)

		err := os.MkdirAll(downDir, os.ModePerm)
		if err != nil {
			logger.Errorf("%v, failed to create restore backup dir", err)
			return fmt.Errorf("create backup file failed")
		}

		param := strings.Join(cfg.XbstreamArgs(downDir), " ")

		cmd := fmt.Sprintf("curl -f %s | %s %s", cfg.BackupServerDownload(file, local), xbstreamCommand, param)

		unpack := exec.Command("bash", "-c", cmd)

		logger.Info(unpack.String())

		var stdout, stderr bytes.Buffer
		unpack.Stdout = &stdout
		unpack.Stderr = &stderr

		if err := unpack.Run(); err != nil {
			logger.Errorf("%v failed to unpack xtrabackup data", err)
			fmt.Println(unpack.String())
			fmt.Println(stdout.String())
			fmt.Println(stderr.String())
			return err
		}

		logger.Info("download backup file" + file + "success")
	}

	logger.Info("download all file success")
	return nil
}

func localBackupNFS(cfg *Config) error {

	for _, path := range cfg.BackupRecordFiles {
		file := GetBackupFilePath(path)
		downDir := GetBackupFileDownloadDir(path)
		downFile := downDir + "/" + GreatdbBackupName
		err := os.MkdirAll(downDir, os.ModePerm)
		if err != nil {
			logger.Errorf("%v, failed to create restore backup dir", err)
			return fmt.Errorf("create backup file failed")
		}
		backupReader, err := os.Open(downFile)
		if err != nil {
			err = fmt.Errorf("failed to open backup stream file, %s", err.Error())
			logger.Error(err.Error())
			return err
		}
		defer backupReader.Close()

		// unpack the backup
		unpack := exec.Command(xbstreamCommand, cfg.XbstreamArgs(downDir)...)
		logger.Info(unpack.String())

		unpack.Stdin = backupReader

		var stdout, stderr bytes.Buffer
		unpack.Stdout = &stdout
		unpack.Stderr = &stderr

		if err := unpack.Run(); err != nil {
			logger.Errorf("%v failed to unpack xtrabackup data", err)
			fmt.Println(unpack.String())
			fmt.Println(stdout.String())
			fmt.Println(stderr.String())
			return err
		}

		logger.Info("download backup file" + file + "success")
	}

	logger.Info("download all file success")
	return nil
}

func downBackupS3(cfg *Config) error {

	// mc config host add xx http://xx xx xx --api s3v4
	mcAddHost := exec.Command(mcCommand, cfg.MCAddHostArgs()...)
	// logger.Info(mcAddHost.String())

	err := mcAddHost.Run()
	if err != nil {
		err = fmt.Errorf("failed to mc add host, %s", err.Error())
		logger.Error(err.Error())
		return err
	}

	for _, path := range cfg.BackupRecordFiles {
		file := GetBackupFilePath(path)
		downDir := GetBackupFileDownloadDir(path)
		s3Path := cfg.MCDownloadAddress(path)
		s3File := s3Path + "/" + GreatdbBackupName
		downFile := downDir + "/" + GreatdbBackupName
		logPath := GetBackupFileDownloadMCLog(path)

		err := os.MkdirAll(downDir, os.ModePerm)
		if err != nil {
			logger.Errorf("%v, failed to create backup dir", err)
			return fmt.Errorf("create backup file failed")
		}

		// bucket 下载
		mcDownload := exec.Command(mcCommand, "cp", s3File, downDir)

		logger.Info(mcDownload.String())

		mcLog, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0755)
		if err != nil {
			err = fmt.Errorf("failed to open mc log file, %s", err.Error())
			logger.Error(err.Error())
			return err
		}
		defer mcLog.Close()
		mcDownload.Stdout = mcLog

		mcErrLog, err := os.OpenFile(GetBackupFileDownloadMCErrorLog(path), os.O_WRONLY|os.O_CREATE|os.O_SYNC, 0755)
		if err != nil {
			err = fmt.Errorf("failed to open mc err log file, %s", err.Error())
			logger.Error(err.Error())
			return err
		}
		defer mcErrLog.Close()
		mcDownload.Stderr = mcErrLog

		if err := mcDownload.Run(); err != nil {
			logger.Errorf("%v failed to download xtrabackup data", err)
			return err
		}

		backupReader, err := os.Open(downFile)
		if err != nil {
			err = fmt.Errorf("failed to open backup stream file, %s", err.Error())
			logger.Error(err.Error())
			return err
		}
		defer backupReader.Close()

		// unpack the backup
		unpack := exec.Command(xbstreamCommand, cfg.XbstreamArgs(downDir)...)
		logger.Info(unpack.String())

		unpack.Stdin = backupReader

		var stdout, stderr bytes.Buffer
		unpack.Stdout = &stdout
		unpack.Stderr = &stderr

		if err := unpack.Run(); err != nil {
			logger.Errorf("%v failed to unpack xtrabackup data", err)
			fmt.Println(unpack.String())
			fmt.Println(stdout.String())
			fmt.Println(stderr.String())
			return err
		}

		logger.Info("download backup file" + file + "success")
	}

	logger.Info("download all file success")
	return nil
}

func restoreBackupFile(cfg *Config) error {
	applyLogOnly := true
	inclDir := ""
	fullDir := GetBackupFileDownloadDir(cfg.BackupRecordFiles[0])
	var stdout, stderr bytes.Buffer

	for ind, path := range cfg.BackupRecordFiles {
		if ind == len(cfg.BackupRecordFiles)-1 {
			applyLogOnly = false
		}
		if ind != 0 {
			inclDir = GetBackupFileDownloadDir(path)
		}
		prepare := exec.Command(xtrabackupCommand, cfg.XtrabackupPrepareArgs(fullDir, applyLogOnly, inclDir)...)

		prepare.Stdout = &stdout
		prepare.Stderr = &stderr
		if err := prepare.Run(); err != nil {
			logger.Errorf("%v failed to prepare xtrabackup", err)
			fmt.Println(prepare.String())
			fmt.Println(stdout.String())
			fmt.Println(stderr.String())
			return err
		}

		logger.Infof("%v success to prepare xtrabackup", path)
	}
	xcopy := exec.Command(xtrabackupCommand, "--move-back", fmt.Sprintf("--target-dir=%s", fullDir), fmt.Sprintf("--datadir=%s", dataDir))
	xcopy.Stdout = &stdout
	xcopy.Stderr = &stderr
	if err := xcopy.Run(); err != nil {
		logger.Errorf("%v failed to xcopy xtrabackup command", err)
		fmt.Println(xcopy.String())
		fmt.Println(stdout.String())
		fmt.Println(stderr.String())
		return err
	}

	logger.Info("restore all file success")
	return nil
}

func prepareDir() error {
	logger.Info("create restore dir")
	paths := []string{dataDir, GreatdbRestorePath}
	for _, path := range paths {
		if err := removeDir(path); err != nil {
			return err
		}
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			logger.Errorf("%v, failed to create backup dir %v", err, path)
			return fmt.Errorf("create backup dir %v failed", path)
		}
	}
	logger.Info("create restore dir success")
	return nil
}

func RunRestore() error {

	// Used to determine if it has been restored
	restoreTagPath := strings.Join([]string{GreatdbDataMountPath, "restore"}, "/")

	_, err := os.Stat(restoreTagPath)
	if err == nil {
		return nil
	}

	if !os.IsNotExist(err) {
		logger.Reason(err).Error("failed to check and restore tags")
	}

	cfg := NewConfig()
	logger.Info("restore begin")
	cfg.XtrabackupTargetDir = GreatdbRestorePath
	if err := prepareDir(); err != nil {
		logger.Errorf("%v, failed to create backup dir", err)
		return err
	}
	if err := downBackupProcess(cfg); err != nil {
		return err
	}

	if err := restoreBackupFile(cfg); err != nil {
		return err
	}

	if err := removeDir(GreatdbRestorePath); err != nil {
		return err
	}
	// mark recovery complete
	file, err := os.Create(restoreTagPath)
	if err == nil {
		file.Write([]byte("1"))
		file.Close()
	}

	logger.Info("restore success")
	return nil
}

func downBackupProcess(cfg *Config) error {
	switch cfg.BackupStorage {
	case v1alpha1.BackupStorageNFS:
		return localBackupNFS(cfg)
	case v1alpha1.BackupStorageUploadServer:
		return downBackupFilesystem(cfg, false)
	case v1alpha1.BackupStorageS3:
		return downBackupS3(cfg)
	default:
		return fmt.Errorf("storage not support")
	}
}
