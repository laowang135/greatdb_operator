package secret

import (
	"context"
	"fmt"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	dblog "greatdb-operator/pkg/utils/log"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
)

const (
	EventMessageCreateClusterSecret          = "The system creates a default secret, but this is not recommended. It is recommended to create a secret yourself"
	EventMessageCreateClusterSecretSucceeded = "Cluster default secret created successfully"
	EventReasonCreateClusterSecret           = "create cluster secret"
	EventMessageUpdateClusterSecret          = "A complete secret should be provided"
)

type SecretManager struct {
	Client   *deps.ClientSet
	Lister   *deps.Listers
	Recorder record.EventRecorder
}

func (ret *SecretManager) Sync(cluster *v1alpha1.GreatDBPaxos) (err error) {
	ns, clusterName, secretName := cluster.Namespace, cluster.Name, cluster.Spec.SecretName
	// If the user has set the secretName, you need to check whether the secret really exists. If it does not exist, the operator should create it
	secret, err := ret.GetSecret(ns, secretName)

	if err != nil {
		return err
	}
	// Already exists, update directly
	if secret != nil {
		if err = ret.UpdateSecret(cluster, secret); err != nil {
			return err
		}
		cluster.Spec.SecretName = secret.GetName()
		return nil
	}

	if err = ret.CreateSecret(cluster); err != nil {
		return err
	}
	if err = ret.UpdateClusterSecretName(ns, clusterName, cluster.Spec.SecretName); err != nil {
		return err
	}

	return nil
}

func (ret SecretManager) GetSecret(ns, name string) (*corev1.Secret, error) {

	if name == "" {
		return nil, nil
	}

	secret, err := ret.Client.KubeClientset.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err == nil {
		return secret, nil
	}

	if k8serrors.IsNotFound(err) {
		dblog.Log.Infof("This secret %s/%s does not exist", ns, name)
		return nil, nil
	}

	dblog.Log.Errorf("Failed to  get secret %s/%s, message: %s", ns, name, err.Error())
	return nil, err
}

// CreateSecret Create a secret for the cluster
func (ret *SecretManager) CreateSecret(cluster *v1alpha1.GreatDBPaxos) error {
	// The cluster starts to clean, and no more resources need to be created
	if !cluster.DeletionTimestamp.IsZero() {
		return nil
	}

	// recorder event
	ret.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventReasonCreateClusterSecret, EventMessageCreateClusterSecret)

	//  a new secret
	secret, err := ret.NewClusterSecret(cluster)
	if err != nil {
		dblog.Log.Errorf("failed to new secret %s", err.Error())
		return err
	}

	cluster.Spec.SecretName = secret.GetName()

	// create secret
	_, err = ret.Client.KubeClientset.CoreV1().Secrets(cluster.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			cluster.Spec.SecretName = secret.GetName()
			dblog.Log.Info(err.Error())
			return nil
		}
		dblog.Log.Errorf("failed to create secret %s/%s , messages: %s", secret.Namespace, secret.Name, err.Error())
		return err
	}

	ret.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventMessageCreateClusterSecretSucceeded, EventMessageCreateClusterSecret)

	return nil
}

// NewClusterSecret Returns the available cluster secret
func (ret SecretManager) NewClusterSecret(cluster *v1alpha1.GreatDBPaxos) (secret *corev1.Secret, err error) {
	clusterName, namespace := cluster.Name, cluster.Namespace
	name := ret.getSecretName(clusterName)
	if err != nil {
		return secret, err
	}
	labels := ret.GetLabels(clusterName)

	data := ret.getDefaultSecretData(cluster)
	owner := resources.GetGreatDBClusterOwnerReferences(clusterName, cluster.UID)

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			Finalizers:      []string{resources.FinalizersGreatDBCluster},
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Data: data,
	}

	return secret, nil
}

// getSecretName Returns the  secret names
func (ret SecretManager) getSecretName(clusterName string) string {
	return fmt.Sprintf("%s-%s", clusterName, "secret")
}

// GetLabels  Return to the default label settings
func (ret SecretManager) GetLabels(name string) (labels map[string]string) {

	labels = make(map[string]string)
	labels[resources.AppKubeNameLabelKey] = resources.AppKubeNameLabelValue
	labels[resources.AppkubeManagedByLabelKey] = resources.AppkubeManagedByLabelValue
	labels[resources.AppKubeInstanceLabelKey] = name
	return

}

// getDefaultSecretData Return to the default secret data
func (ret SecretManager) getDefaultSecretData(cluster *v1alpha1.GreatDBPaxos) (data map[string][]byte) {
	data = make(map[string][]byte)
	rootpwd := resources.RootPasswordDefaultValue
	data[resources.RootPasswordKey] = []byte(rootpwd)
	user, pwd := resources.GetClusterUser(cluster)
	data[resources.ClusterUserKey] = []byte(user)
	data[resources.ClusterUserPasswordKey] = []byte(pwd)
	return
}

// UpdateSecret  Update existing secret
func (ret SecretManager) UpdateSecret(cluster *v1alpha1.GreatDBPaxos, secret *corev1.Secret) error {

	// clean cluster
	if !cluster.DeletionTimestamp.IsZero() {
		if len(secret.Finalizers) == 0 {
			return nil
		}
		patch := `[{"op":"remove","path":"/metadata/finalizers"}]`

		_, err := ret.Client.KubeClientset.CoreV1().Secrets(secret.Namespace).Patch(
			context.TODO(), secret.Name, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
		if err != nil {
			dblog.Log.Errorf("failed to delete finalizers of secret %s/%s,message: %s", secret.Namespace, secret.Name, err.Error())
		}

		return nil

	}

	needUpdate := ret.SetDefaultValue(cluster, secret)

	if !needUpdate {
		dblog.Log.V(3).Infof("Secret %s/%s does not need to be updated", secret.Namespace, secret.Name)
		return nil
	}
	_, err := ret.Client.KubeClientset.CoreV1().Secrets(secret.Namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	if err != nil {
		dblog.Log.Errorf("failed to update secret %s/%s, message: %s", secret.Namespace, secret.Name, err.Error())
		return err
	}
	ret.Recorder.Eventf(secret, corev1.EventTypeWarning, "update secret", EventMessageUpdateClusterSecret)

	return nil
}

// SetDefaultValue Set secret default value
func (ret SecretManager) SetDefaultValue(cluster *v1alpha1.GreatDBPaxos, secret *corev1.Secret) bool {
	needUpdate := false

	if ret.SetDefaultData(secret, cluster) {
		needUpdate = true
	}

	if ret.updateMeta(cluster, secret) {
		needUpdate = true
	}

	return needUpdate
}

// SetDefaultData  Set missing required fields
func (ret SecretManager) SetDefaultData(secret *corev1.Secret, cluster *v1alpha1.GreatDBPaxos) bool {
	needUpdate := false

	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}

	if cluster.Status.Phase == v1alpha1.GreatDBPaxosPending {
		if _, ok := secret.Data[resources.ClusterUserKey]; ok {
			delete(secret.Data, resources.ClusterUserKey)
			needUpdate = true
		}

		if _, ok := secret.Data[resources.ClusterUserPasswordKey]; ok {
			delete(secret.Data, resources.ClusterUserPasswordKey)
			needUpdate = true
		}
	}

	data := ret.getDefaultSecretData(cluster)

	for key, value := range data {
		if _, ok := secret.Data[key]; !ok {
			secret.Data[key] = []byte(value)
			needUpdate = true
		}

	}

	return needUpdate
}

func (ret SecretManager) updateMeta(cluster *v1alpha1.GreatDBPaxos, secret *corev1.Secret) bool {
	needUpdate := false

	// update labels
	if ret.UpdateLabel(cluster.Name, secret) {
		needUpdate = true
	}

	// update ownerReferences
	if secret.OwnerReferences == nil {
		secret.OwnerReferences = make([]metav1.OwnerReference, 0, 1)
	}
	exist := false
	for _, own := range secret.OwnerReferences {
		if own.UID == cluster.UID {
			exist = true
			break
		}
	}
	if !exist {

		owner := resources.GetGreatDBClusterOwnerReferences(cluster.Name, cluster.UID)
		// No need to consider references from other owners
		secret.OwnerReferences = []metav1.OwnerReference{owner}
		needUpdate = true
	}

	return needUpdate

}

func (ret SecretManager) UpdateLabel(clusterName string, secret *corev1.Secret) bool {
	needUpdate := false
	if secret.Labels == nil {
		secret.Labels = map[string]string{}
	}

	labels := ret.GetLabels(clusterName)
	for key, value := range labels {
		if v, ok := secret.Labels[key]; !ok || v != value {
			needUpdate = true
			secret.Labels[key] = value
		}
	}

	return needUpdate

}

// UpdateClusterSecretName update cluster field spec.secretName
func (ret SecretManager) UpdateClusterSecretName(ns, clustername, secretName string) error {

	patch := fmt.Sprintf(`[{"op":"replace","path":"/spec/secretName","value":"%s"}]`, secretName)
	fmt.Println(patch)

	_, err := ret.Client.Clientset.GreatdbV1alpha1().GreatDBPaxoses(ns).Patch(context.Background(), clustername, types.JSONPatchType, []byte(patch), metav1.PatchOptions{})
	if err != nil {
		dblog.Log.Errorf("failed to update field secretName of  cluster %s/%s, message: %s", ns, clustername, err.Error())
		return err
	}

	return nil

}
