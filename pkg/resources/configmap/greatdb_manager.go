package configmap

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/resources/internal"
	internalConfig "greatdb-operator/pkg/resources/internal/config"

	corev1 "k8s.io/api/core/v1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8sresources "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"

	dblog "greatdb-operator/pkg/utils/log"
)

type greatdbConfigManager struct {
	client   deps.ClientSet
	Listers  *deps.Listers
	recorder record.EventRecorder
}

func NewGreatdbConfigManager(client *deps.ClientSet, Listers *deps.Listers, recorder record.EventRecorder) *greatdbConfigManager {

	return &greatdbConfigManager{
		client:   *client,
		recorder: recorder,
		Listers:  Listers,
	}

}

func (greatdb *greatdbConfigManager) Sync(cluster *v1alpha1.GreatDBPaxos) error {

	greatdb.UpdateTargetInstanceToMember(cluster)

	ns, clusterName := cluster.Namespace, cluster.Name
	configmapName := clusterName + resources.ComponentGreatDBSuffix
	configmap, err := greatdb.Listers.ConfigMapLister.ConfigMaps(ns).Get(configmapName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if err := greatdb.createGreatdbConfigmap(cluster); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("failed to sync configmap %s/%s of dbscale, message: %s", ns, configmap, err.Error())
		return err
	}

	if err := greatdb.updateConfigmap(configmap, cluster); err != nil {
		return err
	}

	dblog.Log.Infof("Cluster %s/%s  configmap %s/%s sync succeeded", ns, clusterName, ns, configmapName)
	return nil
}

// createGreatdbConfigmap create configmap of greatdb
func (greatdb *greatdbConfigManager) createGreatdbConfigmap(cluster *v1alpha1.GreatDBPaxos) error {
	// The cluster starts to clean, and no more resources need to be created
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}
	configmap := greatdb.NewGreatdbConfigMap(cluster)
	_, err := greatdb.client.KubeClientset.CoreV1().ConfigMaps(cluster.Namespace).Create(context.TODO(), configmap, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			// The configmap already exists, but for unknown reasons, the operator has not monitored the configmap.
			//  need to add a label to the configmap to ensure that the operator can monitor it
			labels, _ := json.Marshal(configmap.ObjectMeta.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			_, err = greatdb.client.KubeClientset.CoreV1().ConfigMaps(cluster.Namespace).Patch(
				context.TODO(), configmap.Name, types.StrategicMergePatchType, []byte(data), metav1.PatchOptions{})
			if err != nil {
				dblog.Log.Errorf("failed to update the labels of configmap, message: %s", err.Error())
				return err
			}
			return nil

		}
		dblog.Log.Errorf("failed to create greatdb configmap, message: %s", err.Error())
		// record event
		greatdb.recorder.Eventf(cluster, corev1.EventTypeNormal, err.Error(), CreateGreatdbConfigMapFailedReason)
		return err
	}
	name := cluster.Name + resources.ComponentGreatDBSuffix
	dblog.Log.Infof("configmap %s/%s created successfully", cluster.Namespace, name)
	return nil
}

func (greatdb *greatdbConfigManager) NewGreatdbConfigMap(cluster *v1alpha1.GreatDBPaxos) (configmap *corev1.ConfigMap) {
	// Combination of configuration name with suffix
	name := cluster.Name + resources.ComponentGreatDBSuffix
	labels := greatdb.GetLabels(cluster.Name)
	data, err := greatdb.getGreatdbConfigData(cluster)
	if err != nil {
		dblog.Log.Errorf("failed to get config data ,message: %s", err.Error())
		return
	}
	configmap = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  cluster.Namespace,
			Labels:     labels,
			Finalizers: []string{resources.FinalizersGreatDBCluster},
		},
		Data: data,
	}

	return
}

func (greatdb greatdbConfigManager) getGreatdbConfigData(cluster *v1alpha1.GreatDBPaxos) (map[string]string, error) {

	iniParse, err := internalConfig.NewIniParserforByte([]byte(greatdbConfigTemplate))
	if err != nil {
		dblog.Log.Errorf("Failed to instantiate ini configuration parser,message:%s", err.Error())

	}

	// set init config
	if err := greatdb.initParams(iniParse, cluster); err != nil {
		dblog.Log.Reason(err).Error("failed to init config")
		return nil, err
	}

	// Set user configuration to configuration template
	if err = greatdb.addCustomParams(iniParse, cluster); err != nil {
		dblog.Log.Errorf("Failed to add custom params, message: %s", err.Error())
		return nil, err
	}

	if err = greatdb.addAutoTuneParams(iniParse, cluster); err != nil {
		dblog.Log.Errorf("Failed to add tune params, message: %s", err.Error())
		return nil, err
	}

	data := iniParse.SaveToString()
	if data == "" {
		dblog.Log.Error("configuration export data is empty")
		return nil, fmt.Errorf("configuration export data is empty")
	}
	confMap := make(map[string]string, 1)
	confMap["greatdb.cnf"] = data
	return confMap, nil

}

// GetLables Return to the default label settings
func (greatdb *greatdbConfigManager) GetLabels(name string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = name
	labels[resources.AppKubeComponentLabelKey] = resources.AppKubeComponentGreatDB
	return

}

func (greatdb greatdbConfigManager) updateConfigmap(configmap *corev1.ConfigMap, cluster *v1alpha1.GreatDBPaxos) error {

	if !cluster.DeletionTimestamp.IsZero() {
		if len(configmap.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`

		_, err := greatdb.client.KubeClientset.CoreV1().ConfigMaps(configmap.Namespace).Patch(
			context.TODO(), configmap.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of configmap %s/%s,message: %s", configmap.Namespace, configmap.Name, err.Error())
		}

		return nil
	}
	needUpdate := false

	if greatdb.updateConfigmapMeta(configmap, cluster) {
		needUpdate = true
	}

	n, err := greatdb.updateConfigmapData(configmap, cluster)
	if err != nil {
		dblog.Log.Errorf(err.Error())
	}

	if n {
		needUpdate = true

	}

	if !needUpdate {
		return nil
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {

		_, err := greatdb.client.KubeClientset.CoreV1().ConfigMaps(configmap.Namespace).Update(context.TODO(),
			configmap, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}
		upConfigMap, err1 := greatdb.Listers.ConfigMapLister.ConfigMaps(configmap.Namespace).Get(configmap.Name)
		if err1 != nil {
			dblog.Log.Errorf("error getting updated configmap %s/%s from lister,message: %s", configmap.Namespace, configmap.Name, err1.Error())
		} else {
			configmap.ResourceVersion = upConfigMap.ResourceVersion
		}
		dblog.Log.Errorf("failed to update configmap  %s/%s, message: %s", configmap.Namespace, configmap.Name, err.Error())
		return err
	})

	if err != nil {
		dblog.Log.Errorf("failed to update greatDB configmap %s/%s ,message: %s", configmap.Namespace, configmap.Name, err.Error())
		return err
	}
	return nil

}

func (greatdb greatdbConfigManager) updateConfigmapMeta(configmap *corev1.ConfigMap, cluster *v1alpha1.GreatDBPaxos) bool {

	needUpdate := false
	// update labels
	labels := greatdb.GetLabels(cluster.Name)
	if greatdb.updateLabel(configmap, labels) {
		needUpdate = true
	}

	// update Finalizers
	if configmap.Finalizers == nil {
		configmap.Finalizers = make([]string, 0, 1)
	}
	exist := false
	for _, fin := range configmap.Finalizers {
		if fin == resources.FinalizersGreatDBCluster {
			exist = true
		}
	}
	if !exist {
		configmap.Finalizers = append(configmap.Finalizers, resources.FinalizersGreatDBCluster)
		needUpdate = true
	}

	// update OwnerReferences
	if greatdb.updateOwnerReferences(configmap, cluster) {
		needUpdate = true
	}

	return needUpdate
}

func (greatdb greatdbConfigManager) updateLabel(configmap *corev1.ConfigMap, labels map[string]string) bool {
	needUpdate := false

	if configmap.Labels == nil {
		configmap.Labels = make(map[string]string)
	}

	for key, value := range labels {
		if v, ok := configmap.Labels[key]; !ok || v != value {
			configmap.Labels[key] = value
			needUpdate = true
		}
	}

	return needUpdate
}

func (greatdb greatdbConfigManager) updateOwnerReferences(configmap *corev1.ConfigMap, cluster *v1alpha1.GreatDBPaxos) bool {
	needUpdate := false
	owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)

	if configmap.OwnerReferences == nil {
		configmap.OwnerReferences = make([]metav1.OwnerReference, 0, 1)
	}
	exist := false
	for _, own := range configmap.OwnerReferences {
		if own.UID == owner.UID {
			exist = true
			break
		}
	}
	if !exist {
		// No need to consider references from other owners
		configmap.OwnerReferences = []metav1.OwnerReference{owner}
		needUpdate = true
	}

	return needUpdate
}

func (greatdb greatdbConfigManager) updateConfigmapData(configmap *corev1.ConfigMap, cluster *v1alpha1.GreatDBPaxos) (bool, error) {

	needUpdate := false
	if _, ok := configmap.Data["greatdb.cnf"]; !ok {
		return false, fmt.Errorf("greatdb.cnf not exist")
	}
	iniParse, err := internalConfig.NewIniParserforByte([]byte(configmap.Data["greatdb.cnf"]))
	if err != nil {
		return false, fmt.Errorf("ini parse greatdb.cnf err")
	}
	var changeOptions = map[string]string{}
	for newKey, newValue := range cluster.Spec.Config {
		if _, ok := GreatdbFixedOptions[newKey]; ok {
			continue
		}
		if _, ok := GreatdbSupportDynamicOptions[newKey]; ok {
			oldValue, getErr := iniParse.GetKey("mysqld", newKey)
			if getErr != nil {
				dblog.Log.Errorf("Failed to add custom params, message: %s", err.Error())
				continue
			}
			if oldValue.Value() != newValue {
				changeOptions[newKey] = newValue
				needUpdate = true
			}
		}
	}

	err = greatdb.updateGreatdbOption(cluster, changeOptions)
	if err != nil {
		return false, fmt.Errorf("update greatdb option error")
	}

	data, err := greatdb.getGreatdbConfigData(cluster)
	if err != nil {
		dblog.Log.Errorf("failed to get config data ,message: %s", err.Error())
		return false, err
	}

	if !reflect.DeepEqual(configmap.Data, data) {
		needUpdate = true
	}
	configmap.Data = data

	return needUpdate, nil
}

func (greatdb greatdbConfigManager) initParams(config *internalConfig.IniParser, cluster *v1alpha1.GreatDBPaxos) error {
	mysqldSection := config.GetSection("mysqld")
	if mysqldSection == nil {
		return fmt.Errorf("section mysqld not exist")
	}

	groupSeed := ""

	hosts := greatdb.getGreatdbServiceClientUri(cluster)
	// whitelist := ""
	for _, host := range hosts {
		groupSeed += fmt.Sprintf("%s:%d,", host, resources.GroupPort)
		// whitelist += host + ","
	}

	groupSeed = strings.TrimSuffix(groupSeed, ",")
	// whitelist = strings.TrimSuffix(whitelist, ",")

	// loose-group_replication_group_seeds
	// config.SetValue("mysqld", "loose-group_replication_ip_whitelist", whitelist)
	config.SetValue("mysqld", "loose-group_replication_group_seeds", groupSeed)

	// port
	config.SetValue("mysqld", "port", fmt.Sprintf("%d", cluster.Spec.Port))

	return nil
}

func (greatdb greatdbConfigManager) addCustomParams(config *internalConfig.IniParser, cluster *v1alpha1.GreatDBPaxos) error {
	mysqldSection := config.GetSection("mysqld")
	if mysqldSection == nil {
		return fmt.Errorf("section mysqld not exist")
	}
	for newKey, newValue := range cluster.Spec.Config {
		if _, ok := GreatdbFixedOptions[newKey]; ok {
			continue
		}
		config.SetValue("mysqld", newKey, newValue)
	}

	return nil
}

func (greatdb greatdbConfigManager) addAutoTuneParams(config *internalConfig.IniParser, cluster *v1alpha1.GreatDBPaxos) error {
	var memory *k8sresources.Quantity
	var cpu *k8sresources.Quantity
	if res := cluster.Spec.Resources; res.Size() > 0 {
		if _, ok := res.Requests[corev1.ResourceMemory]; ok {
			memory = res.Requests.Memory()
			cpu = res.Requests.Cpu()
		}
		if _, ok := res.Limits[corev1.ResourceMemory]; ok {
			memory = res.Limits.Memory()
			cpu = res.Requests.Cpu()
		}
	}
	if cpu != nil {
		threadConcurrency := cpu.Value() * int64(2)

		mysqldSection := config.GetSection("mysqld")
		if mysqldSection == nil {
			return fmt.Errorf("section mysqld not exist")
		}
		if !mysqldSection.HasKey("innodb_thread_concurrency") {
			threadConcurrencyVal := strconv.FormatInt(threadConcurrency, 10)
			config.SetValue("mysqld", "innodb_thread_concurrency", threadConcurrencyVal)
		}
	}
	if memory == nil {
		return nil
	}

	q := memory
	poolSize := q.Value() * int64(75) / int64(100)
	if q.Value()-poolSize < int64(1000000000) {
		poolSize = q.Value() * int64(50) / int64(100)
	}
	instances := int64(1)                 // default value
	chunkSize := int64(1024 * 1024 * 128) // default value

	// Adjust innodb_buffer_pool_chunk_size
	// If innodb_buffer_pool_size is bigger than 1Gi, innodb_buffer_pool_instances is set to 8.
	// By default, innodb_buffer_pool_chunk_size is 128Mi and innodb_buffer_pool_size needs to be
	// multiple of innodb_buffer_pool_chunk_size * innodb_buffer_pool_instances.
	// More info: https://dev.mysql.com/doc/refman/8.0/en/innodb-buffer-pool-resize.html
	if poolSize > int64(1073741824) {
		instances = 8
		chunkSize = poolSize / instances
		// innodb_buffer_pool_chunk_size can be increased or decreased in units of 1Mi (1048576 bytes).
		// That's why we should strip redundant bytes
		chunkSize -= chunkSize % (1048576)
	}

	// Buffer pool size must always
	// be equal to or a multiple of innodb_buffer_pool_chunk_size * innodb_buffer_pool_instances.
	// If not, this value will be adjusted
	if poolSize%(instances*chunkSize) != 0 {
		poolSize += (instances * chunkSize) - poolSize%(instances*chunkSize)
	}
	mysqldSection := config.GetSection("mysqld")
	if mysqldSection == nil {
		return fmt.Errorf("section mysqld not exist")
	}
	if !mysqldSection.HasKey("innodb_buffer_pool_size") {
		poolSizeVal := strconv.FormatInt(poolSize, 10)
		config.SetValue("mysqld", "innodb_buffer_pool_size", poolSizeVal)

		if !mysqldSection.HasKey("innodb_buffer_pool_chunk_size") {
			chunkSizeVal := strconv.FormatInt(chunkSize, 10)
			config.SetValue("mysqld", "innodb_buffer_pool_chunk_size", chunkSizeVal)
		}
	}

	if !mysqldSection.HasKey("max_connections") {
		divider := int64(12582880)
		if q.Value() < divider {
			return fmt.Errorf("not enough memory set in requests. Must be >= 12Mi")
		}
		maxConnSize := q.Value() / divider
		maxConnSizeVal := strconv.FormatInt(maxConnSize, 10)
		config.SetValue("mysqld", "max_connections", maxConnSizeVal)
	}

	return nil
}

func (greatdb greatdbConfigManager) getGreatdbServiceClientUri(cluster *v1alpha1.GreatDBPaxos) (uris []string) {

	for _, member := range cluster.Status.Member {

		if member.Address != "" {
			uris = append(uris, member.Address)
			continue
		}
		svcName := cluster.Name + resources.ComponentGreatDBSuffix
		host := fmt.Sprintf("%s.%s.%s.svc.%s", member.Name, svcName, cluster.Namespace, cluster.Spec.ClusterDomain)
		// TODO Debug
		// host := resources.GetInstanceFQDN(cluster.Name, member.Name, cluster.Namespace, cluster.Spec.ClusterDomain)
		uris = append(uris, host)
	}

	return uris
}

func (greatdb greatdbConfigManager) updateGreatdbOption(cluster *v1alpha1.GreatDBPaxos, changeOptions map[string]string) (err error) {

	if len(changeOptions) == 0 {
		return nil
	}

	dbVariables := internal.NewDBVariable(changeOptions)
	user, password := resources.GetClusterUser(cluster)
	port := int(cluster.Spec.Port)
	for _, uri := range greatdb.getGreatdbServiceClientUri(cluster) {
		db := internal.NewDBClient()
		err = db.Connect(user, password, uri, port, "mysql")
		if err != nil {
			return fmt.Errorf("update greatdb option error, %v", err)
		}
		defer db.Close()
		err = db.UpdateDBVariables(dbVariables)
		if err != nil {
			return fmt.Errorf("update greatdb option error, %v", err)
		}
	}

	return nil

}

func (greatdb greatdbConfigManager) UpdateTargetInstanceToMember(cluster *v1alpha1.GreatDBPaxos) {

	if cluster.Status.Phase != v1alpha1.GreatDBPaxosPending {
		return
	}

	cluster.Status.Instances = cluster.Spec.Instances
	cluster.Status.TargetInstances = cluster.Spec.Instances

	if cluster.Status.Member == nil {
		cluster.Status.Member = make([]v1alpha1.MemberCondition, 0)
	}

	num := len(cluster.Status.Member)

	if num >= int(cluster.Status.TargetInstances) {
		return
	}
	index := GetNextIndex(cluster.Status.Member)

	for num < int(cluster.Status.TargetInstances) {
		name := fmt.Sprintf("%s%s-%d", cluster.Name, resources.ComponentGreatDBSuffix, index)
		cluster.Status.Member = append(cluster.Status.Member, v1alpha1.MemberCondition{
			Name:       name,
			Index:      index,
			CreateType: v1alpha1.InitCreateMember,
			PvcName:    name,
		})
		num++
		index += 1

	}

}
