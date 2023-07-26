package service

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/types"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	dblog "greatdb-operator/pkg/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
)

type ServiceManager struct {
	Client   *deps.ClientSet
	Listers  *deps.Listers
	Recorder record.EventRecorder
}

// Sync  start sync service
func (svc *ServiceManager) Sync(cluster *v1alpha1.GreatDBPaxos) error {

	// Synchronize GreatDB service
	if err := svc.SyncGreatDBHeadlessService(cluster); err != nil {
		dblog.Log.Errorf("failed to synchronize DBscaleHeadlessService, message: %s", err.Error())
		return err
	}

	// Synchronize GreatDB read service
	if err := svc.SyncGreatDBReadService(cluster); err != nil {
		dblog.Log.Errorf("failed to synchronize GreatDB read service, message: %s", err.Error())
		return err
	}

	// Synchronize GreatDB write service
	if err := svc.SyncGreatDBWriteService(cluster); err != nil {
		dblog.Log.Errorf("failed to synchronize GreatDB write service, message: %s", err.Error())
		return err
	}

	// Synchronize greatdb cluster external communication services
	if err := svc.SyncDashboardService(cluster); err != nil {
		dblog.Log.Errorf("failed to synchronize dashboard service, message: %s", err.Error())
		return err
	}

	return nil
}

func (svc *ServiceManager) SyncGreatDBReadService(cluster *v1alpha1.GreatDBPaxos) error {

	ns, clusterName := cluster.Namespace, cluster.Name
	serviceName := clusterName + resources.ServiceRead
	service, err := svc.Listers.ServiceLister.Services(ns).Get(serviceName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// The cluster starts to clean, and no more resources need to be created
			if !cluster.DeletionTimestamp.IsZero() {
				return nil
			}
			if err := svc.createGreatDBReadOrWriteService(cluster, GreatDBServiceRead); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("Failed to get service resource %s/%s from cache ,message: %s", ns, serviceName, err.Error())
		return err
	}
	newService := service.DeepCopy()
	if err = svc.updateGreatDBReadOrWriteService(newService, cluster, GreatDBServiceRead); err != nil {
		return err
	}
	return nil
}

// SyncGreatDBWriteService Synchronize the services of GreatDB
func (svc *ServiceManager) SyncGreatDBWriteService(cluster *v1alpha1.GreatDBPaxos) error {

	ns, clusterName := cluster.Namespace, cluster.Name
	serviceName := clusterName + resources.ServiceWrite
	service, err := svc.Listers.ServiceLister.Services(ns).Get(serviceName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// The cluster starts to clean, and no more resources need to be created
			if !cluster.DeletionTimestamp.IsZero() {
				return nil
			}
			if err := svc.createGreatDBReadOrWriteService(cluster, GreatDBServiceWrite); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("Failed to get service resource %s/%s from cache ,message: %s", ns, serviceName, err.Error())
		return err
	}
	newService := service.DeepCopy()
	if err = svc.updateGreatDBReadOrWriteService(newService, cluster, GreatDBServiceWrite); err != nil {
		return err
	}
	return nil
}

func (svc ServiceManager) createGreatDBReadOrWriteService(cluster *v1alpha1.GreatDBPaxos, greatdbSvcType GreatDBServiceType) error {
	serviceName := cluster.Name + resources.ServiceRead
	if greatdbSvcType == GreatDBServiceWrite {
		serviceName = cluster.Name + resources.ServiceWrite
	}

	labels := svc.getGreatDBServiceLabels(cluster, greatdbSvcType)
	owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	service := svc.NewGreatDBReadOrWriteService(serviceName, labels, owner, cluster, greatdbSvcType)
	_, err := svc.Client.KubeClientset.CoreV1().Services(cluster.Namespace).Create(context.TODO(), service, metav1.CreateOptions{})
	if err != nil {
		// If the service already exists, try to update it
		if k8serrors.IsAlreadyExists(err) {

			labelsData, _ := json.Marshal(labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labelsData)
			if err = svc.PatchService(cluster.Namespace, serviceName, data); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("failed to create service %s, message: %s", serviceName, err.Error())
		return err
	}

	return nil
}

// NewGreatDBReadOrWriteService Returns a service instance of GreatDB
func (svc ServiceManager) NewGreatDBReadOrWriteService(servicename string, labels map[string]string,
	owner metav1.OwnerReference, cluster *v1alpha1.GreatDBPaxos, greatdbSvcType GreatDBServiceType) (service *corev1.Service) {

	svcType := corev1.ServiceTypeClusterIP

	port := cluster.Spec.Port

	if cluster.Spec.Service.Type != "" {
		svcType = cluster.Spec.Service.Type
	}

	ports := make([]corev1.ServicePort, 0)

	switch cluster.Spec.Service.Type {
	case corev1.ServiceTypeClusterIP, corev1.ServiceTypeLoadBalancer:
		ports = append(ports, corev1.ServicePort{Name: "client", Port: port, Protocol: corev1.ProtocolTCP, TargetPort: intstr.IntOrString{IntVal: port}})
	case corev1.ServiceTypeNodePort:
		var nport int32
		if greatdbSvcType == GreatDBServiceRead {
			nport = cluster.Spec.Service.ReadPort
		}
		if greatdbSvcType == GreatDBServiceWrite {
			nport = cluster.Spec.Service.WritePort
		}

		ports = append(ports, corev1.ServicePort{
			Name:       "client",
			Port:       port,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.IntOrString{IntVal: port},
			NodePort:   nport,
		})

	}

	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            servicename,
			Namespace:       cluster.Namespace,
			Labels:          labels,
			Finalizers:      []string{resources.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     svcType,
			Ports:    ports,
		},
	}
	return

}

// getGreatDBServiceLabels Returns the label of the GreatDB service
func (svc ServiceManager) getGreatDBServiceLabels(cluster *v1alpha1.GreatDBPaxos, svcType GreatDBServiceType) map[string]string {

	labels := make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppKubeInstanceLabelKey] = cluster.Name
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeComponentLabelKey] = resources.AppKubeComponentGreatDB

	switch svcType {
	case GreatDBServiceRead:
		if !cluster.Spec.PrimaryReadable {
			labels[resources.AppKubeGreatDBRoleLabelKey] = string(v1alpha1.MemberRoleSecondary)
		}
		labels[resources.AppKubeServiceReadyLabelKey] = resources.AppKubeServiceReady

	case GreatDBServiceWrite:
		labels[resources.AppKubeGreatDBRoleLabelKey] = string(v1alpha1.MemberRolePrimary)
		labels[resources.AppKubeServiceReadyLabelKey] = resources.AppKubeServiceReady

	}

	return labels
}

// updateGreatDBReadOrWriteService update GreatDB service If the service is modified manually, restore the service to the normal state
func (svc ServiceManager) updateGreatDBReadOrWriteService(service *corev1.Service, cluster *v1alpha1.GreatDBPaxos, greatdbSvcType GreatDBServiceType) error {
	if !cluster.DeletionTimestamp.IsZero() {
		if len(service.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`

		_, err := svc.Client.KubeClientset.CoreV1().Services(service.Namespace).Patch(
			context.TODO(), service.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of secret %s/%s,message: %s", service.Namespace, service.Name, err.Error())
		}

		return nil
	}

	needUpdate := false

	if service.Spec.Type != corev1.ServiceTypeClusterIP {
		service.Spec.Type = corev1.ServiceTypeClusterIP
		needUpdate = true
	}
	// Prevent labels from being deleted by mistake
	labels := svc.getGreatDBServiceLabels(cluster, greatdbSvcType)
	if svc.updateServiceLabel(service, labels) {
		needUpdate = true
	}

	if svc.updateServiceSelector(service, labels) {
		needUpdate = true
	}
	port := cluster.Spec.Port
	ports := make([]corev1.ServicePort, 0)

	switch cluster.Spec.Service.Type {
	case corev1.ServiceTypeClusterIP, corev1.ServiceTypeLoadBalancer:
		ports = append(ports, corev1.ServicePort{Name: "client", Port: port, Protocol: corev1.ProtocolTCP, TargetPort: intstr.IntOrString{IntVal: port}})

		if service.Spec.Type != cluster.Spec.Service.Type {
			service.Spec.Type = cluster.Spec.Service.Type
			needUpdate = true
		}
	case corev1.ServiceTypeNodePort:
		var nport int32
		if service.Spec.Type != cluster.Spec.Service.Type {
			service.Spec.Type = cluster.Spec.Service.Type
			needUpdate = true
		}
		if greatdbSvcType == GreatDBServiceRead {
			nport = cluster.Spec.Service.ReadPort
		}
		if greatdbSvcType == GreatDBServiceWrite {
			nport = cluster.Spec.Service.WritePort
		}

		ports = append(ports, corev1.ServicePort{
			Name:       "client",
			Port:       port,
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.IntOrString{IntVal: port},
			NodePort:   nport,
		})

	}
	if svc.updateServicePort(service, ports) {
		needUpdate = true
	}

	// update ownerReferences
	if svc.updateOwnerReferences(cluster.Name, cluster.UID, service) {
		needUpdate = true
	}

	if !needUpdate {
		return nil
	}

	if err := svc.updateService(service); err != nil {
		return err
	}
	return nil
}

// SyncGreatDBHeadlessService Synchronize the headless services of dbscale
func (svc *ServiceManager) SyncGreatDBHeadlessService(cluster *v1alpha1.GreatDBPaxos) error {

	ns := cluster.Namespace
	serviceName := cluster.GetName() + resources.ComponentGreatDBSuffix

	service, err := svc.Listers.ServiceLister.Services(ns).Get(serviceName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// The cluster starts to clean, and no more resources need to be created
			if !cluster.DeletionTimestamp.IsZero() {
				return nil
			}
			if err := svc.createGreatDBHeadlessService(cluster, serviceName); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("Failed to get service resource %s/%s from cache ,message: %s", ns, serviceName, err.Error())
		return err
	}

	newService := service.DeepCopy()
	if err = svc.updateGreatDBHeadlessService(newService, cluster); err != nil {
		return err
	}

	return nil
}

func (svc ServiceManager) createGreatDBHeadlessService(cluster *v1alpha1.GreatDBPaxos, serviceName string) error {

	labels := svc.getGreatDBServiceLabels(cluster, GreatDBServiceHeadless)
	owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
	service := svc.NewGreatDBHeadlessService(serviceName, cluster.Namespace, cluster.Spec.Port, labels, owner)

	_, err := svc.Client.KubeClientset.CoreV1().Services(cluster.Namespace).Create(context.TODO(), service, metav1.CreateOptions{})

	if err != nil {
		// If the service already exists, try to update it
		if k8serrors.IsAlreadyExists(err) {

			labelsData, _ := json.Marshal(labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labelsData)
			if err = svc.PatchService(cluster.Namespace, serviceName, data); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("failed to create service %s, message: %s", serviceName, err.Error())
		return err
	}

	return nil
}

// updateGreatDBHeadlessService If the service is modified manually, restore the service to the normal state
func (svc ServiceManager) updateGreatDBHeadlessService(service *corev1.Service, cluster *v1alpha1.GreatDBPaxos) error {
	if !cluster.DeletionTimestamp.IsZero() {
		if len(service.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`

		_, err := svc.Client.KubeClientset.CoreV1().Services(service.Namespace).Patch(
			context.TODO(), service.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of service %s/%s,message: %s", service.Namespace, service.Name, err.Error())
		}

		return nil
	}

	needUpdate := false

	if service.Spec.ClusterIP != "None" {
		service.Spec.ClusterIP = "None"
		needUpdate = true
	}

	if service.Spec.Type != corev1.ServiceTypeClusterIP {
		service.Spec.Type = corev1.ServiceTypeClusterIP
		needUpdate = true
	}
	// Prevent labels from being deleted by mistake
	labels := svc.getGreatDBServiceLabels(cluster, GreatDBServiceHeadless)
	if svc.updateServiceLabel(service, labels) {
		needUpdate = true
	}

	if svc.updateServiceSelector(service, labels) {
		needUpdate = true
	}

	// Ensure that the port meets the latest setting
	ports := []corev1.ServicePort{
		{Name: "client", Port: cluster.Spec.Port, Protocol: corev1.ProtocolTCP, TargetPort: intstr.IntOrString{IntVal: cluster.Spec.Port}},
		{Name: "group", Port: resources.GroupPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.IntOrString{IntVal: resources.GroupPort}},
	}
	if svc.updateServicePort(service, ports) {
		needUpdate = true
	}
	if svc.updateOwnerReferences(cluster.Name, cluster.UID, service) {
		needUpdate = true
	}

	if !needUpdate {
		return nil
	}
	if err := svc.updateService(service); err != nil {
		return err
	}
	return nil
}

// NewDBscaleHeadlessService Returns a headless service instance of dbScale
func (svc ServiceManager) NewGreatDBHeadlessService(servicename, namespace string, port int32, labels map[string]string, owner metav1.OwnerReference) (service *corev1.Service) {

	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            servicename,
			Namespace:       namespace,
			Labels:          labels,
			Finalizers:      []string{resources.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: corev1.ServiceSpec{
			Selector:  labels,
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{Name: "client", Port: port, Protocol: corev1.ProtocolTCP, TargetPort: intstr.IntOrString{IntVal: port}},
				{Name: "group", Port: resources.GroupPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.IntOrString{IntVal: resources.GroupPort}},
			},
		},
	}
	return
}

// updateServiceLabel update service label
func (svc ServiceManager) updateServiceLabel(service *corev1.Service, labels map[string]string) bool {
	needUpdate := false

	if service.Labels == nil {
		service.Labels = make(map[string]string)
	}

	for key, value := range labels {
		if v, ok := service.Labels[key]; !ok || v != value {
			service.Labels[key] = value
			needUpdate = true
		}
	}

	return needUpdate
}

// updateServiceSelector Update Load Selector
func (svc ServiceManager) updateServiceSelector(service *corev1.Service, labels map[string]string) bool {
	needUpdate := false

	if !reflect.DeepEqual(service.Spec.Selector, labels) {
		service.Spec.Selector = labels
		needUpdate = true
	}

	return needUpdate
}

// updateServicePort Supplement · missing ports, replace conflicting ports
func (svc ServiceManager) updateServicePort(service *corev1.Service, ports []corev1.ServicePort) bool {
	needUpdate := false
	for _, port := range ports {
		index, replace := getPortIndex(service, port)
		if index == -1 {
			service.Spec.Ports = append(service.Spec.Ports, port)
			needUpdate = true
		}

		if replace {
			service.Spec.Ports[index] = port
			needUpdate = true
		}

	}
	return needUpdate
}

func (ServiceManager) updateOwnerReferences(clusterName string, clusterUID types.UID, service *corev1.Service) bool {
	needUpdate := false
	// update ownerReferences
	if service.OwnerReferences == nil {
		service.OwnerReferences = make([]metav1.OwnerReference, 0, 1)
	}
	exist := false
	for _, own := range service.OwnerReferences {
		if own.UID == clusterUID {
			exist = true
			break
		}
	}
	if !exist {
		owner := resources.GetGreatDBClusterOwnerReferences(clusterName, clusterUID)
		// No need to consider references from other owners
		service.OwnerReferences = []metav1.OwnerReference{owner}
		needUpdate = true
	}
	return needUpdate

}

func (svc ServiceManager) PatchService(ns, serviceName, patch string) error {

	_, err := svc.Client.KubeClientset.CoreV1().Services(ns).Patch(context.TODO(), serviceName, types.StrategicMergePatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		dblog.Log.Errorf("failed to path service %s/%s,message: %s", ns, serviceName, err.Error())
		return err
	}
	return nil
}

func (svc ServiceManager) updateService(service *corev1.Service) error {

	ns, name := service.Namespace, service.Name
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {

		_, err := svc.Client.KubeClientset.CoreV1().Services(service.Namespace).Update(context.TODO(), service, metav1.UpdateOptions{})
		if err == nil {
			return nil
		}

		if k8serrors.IsConflict(err) {
			updateService, err1 := svc.Listers.ServiceLister.Services(ns).Get(name)
			if err1 != nil {
				dblog.Log.Errorf("failed to get service %s/%s, message: %s", ns, name, err1.Error())
				return nil
			} else {
				service.ResourceVersion = updateService.ResourceVersion
			}
		}

		dblog.Log.Errorf(err.Error())
		return err
	})

	if err != nil {
		dblog.Log.Errorf("failed to update  service %s/%s, message: %s", ns, name, err.Error())
	}

	return err
}

// getPortIndex Find out whether to add or replace ports from existing ports
// int: Port Index 【 -1 ：non-existent】
// bool: Replace Port
func getPortIndex(service *corev1.Service, port corev1.ServicePort) (int, bool) {

	for index := 0; index < len(service.Spec.Ports); index++ {

		if service.Spec.Ports[index].Name != port.Name && service.Spec.Ports[index].Port != port.Port {
			continue
		}
		if service.Spec.Ports[index].Protocol == port.Protocol && service.Spec.Ports[index].TargetPort.String() == port.TargetPort.String() {
			return index, false
		} else {
			return index, true
		}

	}
	return -1, false
}




// SyncDashboardService Synchronize the services of dashboard
func (svc *ServiceManager) SyncDashboardService(cluster *v1alpha1.GreatDBPaxos) error {

	ns, clusterName := cluster.Namespace, cluster.Name
	serviceName := cluster.GetName() + resources.ComponentDashboardSuffix

	service, err := svc.Listers.ServiceLister.Services(ns).Get(serviceName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// The cluster starts to clean, and no more resources need to be created
			if !cluster.DeletionTimestamp.IsZero() {
				return nil
			}
			if err := svc.createDashboardService(ns, clusterName, serviceName, cluster.UID); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("Failed to get service resource %s/%s from cache ,message: %s", ns, serviceName, err.Error())
		return err
	}

	newservice := service.DeepCopy()
	if err = svc.updateDashboardService(newservice, cluster); err != nil {
		return err
	}
	return nil
}



func (svc ServiceManager) createDashboardService(ns, clusterName, serviceName string, clusterUID types.UID) error {

	labels := svc.getServiceLabels(clusterName,resources.AppKubeComponentDashboard)
	owner := resources.GetGreatDBClusterOwnerReferences(clusterName, clusterUID)
	service := svc.NewDashboardService(serviceName, ns, labels, owner)

	_, err := svc.Client.KubeClientset.CoreV1().Services(ns).Create(context.TODO(), service, metav1.CreateOptions{})

	if err != nil {
		// If the service already exists, try to update it
		if k8serrors.IsAlreadyExists(err) {
			labels, _ := json.Marshal(service.ObjectMeta.Labels)
			data := fmt.Sprintf(`{"metadata":{"labels":%s}}`, labels)
			if err = svc.PatchService(ns, serviceName, data); err != nil {
				return err
			}
			return nil
		}
		dblog.Log.Errorf("failed to create service %s, message: %s", serviceName, err.Error())
		return err
	}

	return nil
}

// NewDashboardService Returns a service instance of Dashboard
func (svc ServiceManager) NewDashboardService(servicename, namespace string, labels map[string]string, owner metav1.OwnerReference) (service *corev1.Service) {
	service = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            servicename,
			Namespace:       namespace,
			Labels:          labels,
			Finalizers:      []string{resources.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{Name: "client", Port: 8080, Protocol: corev1.ProtocolTCP, TargetPort: intstr.IntOrString{IntVal: 8080}},
			},
		},
	}
	return
}

// updateDashboardService If the service is modified manually, restore the service to the normal state
func (svc ServiceManager) updateDashboardService(service *corev1.Service, cluster *v1alpha1.GreatDBPaxos) error {
	if !cluster.DeletionTimestamp.IsZero() {
		if len(service.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`

		_, err := svc.Client.KubeClientset.CoreV1().Services(service.Namespace).Patch(
			context.TODO(), service.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of secret %s/%s,message: %s", service.Namespace, service.Name, err.Error())
		}

		return nil
	}
	needUpdate := false

	if service.Spec.Type != corev1.ServiceTypeClusterIP {
		service.Spec.Type = corev1.ServiceTypeClusterIP
		needUpdate = true
	}
	// Prevent labels from being deleted by mistake
	labels := svc.getServiceLabels(cluster.Name,resources.AppKubeComponentDashboard)
	if svc.updateServiceLabel(service, labels) {
		needUpdate = true
	}

	if svc.updateServiceSelector(service, labels) {
		needUpdate = true
	}

	// Ensure that the port meets the latest setting
	ports := []corev1.ServicePort{
		{Name: "client", Port: 8080, Protocol: corev1.ProtocolTCP, TargetPort: intstr.IntOrString{IntVal: 8080}},
	}
	if svc.updateServicePort(service, ports) {
		needUpdate = true
	}

	// update ownerReferences
	if svc.updateOwnerReferences(cluster.Name, cluster.UID, service) {
		needUpdate = true
	}

	if !needUpdate {
		return nil
	}

	if err := svc.updateService(service); err != nil {
		return err
	}

	return nil
}

func (svc ServiceManager) getServiceLabels(clusterName,componentName string) map[string]string {

	labels := make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = clusterName
	labels[resources.AppKubeComponentLabelKey] = componentName

	return labels
}
