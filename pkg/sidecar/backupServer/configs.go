/*
Copyright 2019 Pressinfra SRL

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backupServer

import (
	"fmt"
	"os"
	"path"
	"strings"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	dblog "greatdb-operator/pkg/utils/log"
)

var logger = dblog.Log

// Config contains information related with the pod.
type Config struct {
	// Hostname represents the pod hostname
	Hostname string
	// ClusterName is the MySQL cluster name
	ClusterName string
	// Namespace represents the namespace where the pod is in
	Namespace string

	// backup user and password for http endpoint
	BackupUser     string
	BackupPassword string
	BackupHost     string
	BackupPort     string

	// BackupCompressCommand is a command to use for compressing the backup.
	BackupCompressCommand []string

	// BackupDecompressCommand is a command to use for decompressing the backup.
	BackupDecompressCommand []string

	// XbstreamExtraArgs is a list of extra command line arguments to pass to xbstream.
	XbstreamExtraArgs []string

	// XtrabackupExtraArgs is a list of extra command line arguments to pass to xtrabackup.
	XtrabackupExtraArgs []string

	// XtrabackupPrepareExtraArgs is a list of extra command line arguments to pass to xtrabackup
	// during --prepare.
	XtrabackupPrepareExtraArgs []string

	// XtrabackupTargetDir is a backup destination directory for xtrabackup.
	XtrabackupTargetDir string

	// backupnode start address.
	BackupServerAddress string
	BackupServerPort    string

	// Destination start address.
	BackupDestination string

	BackupType string

	BackupStorage v1alpha1.BackupStorageType

	BackupResource v1alpha1.BackupResourceType

	// s3
	BackupS3Bucket      string
	BackupS3EndpointURL string
	BackupS3AccessKey   string
	BackupS3SecretKey   string

	// upload server config
	UploadServerAddress string
	UploadServerPort    string

	// backup record info
	BackupIncFromLsn  string
	BackupRecordFiles []string
}

func (cfg *Config) GetBackupDir() string {
	return cfg.XtrabackupTargetDir
}

func (cfg *Config) GetBackupFile() string {
	return cfg.GetBackupDir() + "/" + GreatdbBackupName
}

func (cfg *Config) GetBackupLogFile() string {
	return cfg.GetBackupDir() + "/backup.log"
}

// XtrabackupArgs returns a complete set of xtrabackup arguments.
func (cfg *Config) XtrabackupArgs() []string {
	// xtrabackup --backup <args> --target-dir=<backup-dir> <extra-args>
	xtrabackupArgs := []string{
		"--backup",
		"--stream=xbstream",
		fmt.Sprintf("--extra-lsndir=%s", cfg.XtrabackupTargetDir),
		fmt.Sprintf("--host=%s", cfg.BackupHost),
		fmt.Sprintf("--port=%s", cfg.BackupPort),
		fmt.Sprintf("--datadir=%s", dataDir),
		fmt.Sprintf("--user=%s", cfg.BackupUser),
		fmt.Sprintf("--password=%s", cfg.BackupPassword),
		fmt.Sprintf("--target-dir=%s", cfg.XtrabackupTargetDir),
	}

	if cfg.BackupIncFromLsn != "" {
		xtrabackupArgs = append(xtrabackupArgs, fmt.Sprintf("--incremental-lsn=%s", cfg.BackupIncFromLsn))
	}

	return append(xtrabackupArgs, cfg.XtrabackupExtraArgs...)
}

// XbstreamArgs returns a complete set of xbstream arguments.
func (cfg *Config) XbstreamArgs(dataDir string) []string {
	// xbstream --extract --directory=<mysql-data-dir> <extra-args>
	xbstreamArgs := []string{"--extract", fmt.Sprintf("--directory=%s", dataDir)}
	return append(xbstreamArgs, cfg.XbstreamExtraArgs...)
}

// XtrabackupPrepareArgs returns a complete set of xtrabackup arguments during --prepare.
func (cfg *Config) XtrabackupPrepareArgs(dataDir string, applyLogOnly bool, incDir string) []string {
	// xtrabackup --prepare --target-dir=<mysql-data-dir> <extra-args>
	xtrabackupPrepareArgs := []string{"--prepare", fmt.Sprintf("--target-dir=%s", dataDir)}
	if applyLogOnly {
		xtrabackupPrepareArgs = append(xtrabackupPrepareArgs, "--apply-log-only")
	}
	if incDir != "" {
		xtrabackupPrepareArgs = append(xtrabackupPrepareArgs,
			fmt.Sprintf("--incremental-dir=%s", incDir))
	}
	return append(xtrabackupPrepareArgs, cfg.XtrabackupPrepareExtraArgs...)
}

// XtrabackupArgs returns a complete set of mc arguments.
func (cfg *Config) MCAddHostArgs() []string {
	// mc config host add greatdb http://xx xx xxx --api s3v4
	mcArgs := []string{
		"config",
		"host",
		"add",
		"greatdb",
		cfg.BackupS3EndpointURL,
		cfg.BackupS3AccessKey,
		cfg.BackupS3SecretKey,
		"--api",
		"s3v4",
	}
	return mcArgs
}

func (cfg *Config) MCBucket() string {
	// greatdb/backup-bucket/cluster1/backup/full20220101/backup.stream
	mcArgs := fmt.Sprintf("%s/%s", "greatdb", cfg.BackupS3Bucket)
	return mcArgs
}

func (cfg *Config) MCAddress() string {
	// greatdb/backup-bucket/cluster1/backup/full20220101/backup.stream
	mcArgs := fmt.Sprintf("%s%s", cfg.MCBucket(), cfg.GetBackupDir())
	return mcArgs
}

func (cfg *Config) MCDownloadAddress(path string) string {
	return fmt.Sprintf("%s/backup/%s", cfg.MCBucket(), path)
}

func (cfg *Config) GetMCLog() string {
	return cfg.GetBackupDir() + "/mc.log"
}

func (cfg *Config) GetMCErrorLog() string {
	return cfg.GetBackupDir() + "/mc_error.log"
}

func (cfg *Config) GetClusterBackupLog() string {
	return cfg.GetBackupDir() + "/backup.log"
}

// BackupServer returns a url to use for backup.
func (cfg *Config) BackupServerUrl() string {
	return fmt.Sprintf("http://%s:%s%s", cfg.BackupServerAddress, cfg.BackupServerPort, serverBackupEndpoint)
}

func (cfg *Config) BackupUrl() string {
	return fmt.Sprintf("http://%s:%s%s", cfg.BackupServerAddress, cfg.BackupServerPort, serverBackupEndpoint)
}

// BackupServer returns a url to use for backup.
func (cfg *Config) BackupServerDownload(path string, local bool) string {
	host := cfg.BackupServerAddress
	if !local {
		host = cfg.UploadServerAddress
	}
	return fmt.Sprintf("http://%s:%s%s/%s", host, cfg.BackupServerPort, ServerBackupDownEndpoint, path)
}

// NewConfig returns a pointer to Config configured from environment variables
func NewConfig() *Config {
	cfg := &Config{
		Hostname:  getEnvValue("HOSTNAME"),
		Namespace: getEnvValue("NAMESPACE"),

		ClusterName:    getEnvValue("ClusterName"),
		BackupUser:     getEnvValue("ClusterUser"),
		BackupPassword: getEnvValue("ClusterUserPassword"),
		BackupHost:     getEnvValue("BackupHost"),
		BackupPort:     getEnvValue("BackupPort"),

		BackupCompressCommand:      strings.Fields(getEnvValue("BACKUP_COMPRESS_COMMAND")),
		BackupDecompressCommand:    strings.Fields(getEnvValue("BACKUP_DECOMPRESS_COMMAND")),
		XbstreamExtraArgs:          strings.Fields(getEnvValue("XBSTREAM_EXTRA_ARGS")),
		XtrabackupExtraArgs:        strings.Fields(getEnvValue("XTRABACKUP_EXTRA_ARGS")),
		XtrabackupPrepareExtraArgs: strings.Fields(getEnvValue("XTRABACKUP_PREPARE_EXTRA_ARGS")),
		XtrabackupTargetDir:        getEnvValue("XTRABACKUP_TARGET_DIR"),

		BackupServerAddress: getEnvValue("BackupServerAddress"),
		BackupServerPort:    getEnvValue("BackupServerPort"),
		BackupDestination:   getEnvValue("BackupDestination"),
		BackupType:          getEnvValue("BackupType"),
		BackupStorage:       v1alpha1.BackupStorageType(getEnvValue("BackupStorage")),
		BackupResource:      v1alpha1.BackupResourceType(getEnvValue("BackupResource")),

		// s3 config
		BackupS3Bucket:      getEnvValue("BackupS3Bucket"),
		BackupS3EndpointURL: getEnvValue("BackupS3EndpointURL"),
		BackupS3AccessKey:   getEnvValue("BackupS3AccessKey"),
		BackupS3SecretKey:   getEnvValue("BackupS3SecretKey"),

		// upload server config
		UploadServerAddress: getEnvValue("UploadServerAddress"),
		UploadServerPort:    getEnvValue("UploadServerPort"),

		// backup record info
		BackupIncFromLsn:  getEnvValue("BackupIncFromLsn"),
		BackupRecordFiles: strings.Split(getEnvValue("BackupRecordFiles"), ","),
	}

	return cfg
}

func getEnvValue(key string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		logger.Infof("environment is not set key %s", key)
	}

	return value
}

type UploadServerConfig struct {
	Address string `json:"address,omitempty"`
	Port    string `json:"port,omitempty"`
}

type S3Config struct {
	Bucket       string `json:"bucket"`
	Region       string `json:"region,omitempty"`
	EndpointURL  string `json:"endpointUrl,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
	AccessKey    string `json:"accessKey,omitempty"`
	SecretKey    string `json:"secretKey,omitempty"`
}

type BackupConfig struct {
	Destination    string                      `json:"Destination"`
	IncFromLsn     string                      `json:"IncFromLsn"`
	StorageType    v1alpha1.BackupStorageType  `json:"StorageType"`
	BackupResource v1alpha1.BackupResourceType `json:"BackupResource"`
	S3             S3Config                    `json:"s3,omitempty"`
	UploadServer   UploadServerConfig          `json:"UploadServerConfig,omitempty"`
}

func (cfg *BackupConfig) GetBackupDir(ns string) string {
	return path.Join(GreatdbBackupDataMountPath, ns, cfg.Destination)
}

type BackupInfoRequest struct {
	BackupResource string `json:"backupResource"`
	Name           string `json:"name"`
}

func (req *BackupInfoRequest) GetBackupInfoFile(ns string) string {
	return path.Join("/backup/", ns, req.Name, "/xtrabackup_checkpoints")
}

type BackupInfoResponse struct {
	BackupType string `json:"backupType,omitempty"`
	FromLsn    string `json:"fromLsn,omitempty"`
	ToLsn      string `json:"toLsn,omitempty"`
	LastLsn    string `json:"lastLsn,omitempty"`
}

func GetBackupFilePath(path string) string {
	return path + "/" + GreatdbBackupName
}

func GetBackupFileDownloadDir(dir string) string {
	return path.Join(GreatdbRestorePath, dir)
}

func GetNFSBackupFilePath(dir string) string {
	return path.Join(GreatdbRestorePath, dir)
}

func GetBackupFileDownloadMCLog(dir string) string {
	return path.Join(GreatdbRestorePath, dir) + "/mc.log"
}

func GetBackupFileDownloadMCErrorLog(dir string) string {
	return path.Join(GreatdbRestorePath, dir) + "/mc_error.log"
}

func GetRestoreSourceFilePath(ns, file string) string {
	return path.Join(GreatdbBackupDataMountPath, ns, file)
}
