package admitters

import (
	"context"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	"greatdb-operator/pkg/utils/tools"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/client-go/kubernetes"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"
)

type GreatDBBackupRecordCreateAdmitter struct {
	KubeClient    kubernetes.Interface
	GreatDBClient versioned.Interface
}

func (admit GreatDBBackupRecordCreateAdmitter) Admit(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	record, _, err := utils.GetGreatDBBackupRecordFromAdmissionReview(ar)
	if err != nil {
		return utils.ToAdmissionResponseError(err)
	}
	causes := ValidatingGreatDBBackupRecordSpec(k8sfield.NewPath("spec"), record.Namespace, record.Spec, admit.GreatDBClient)

	return utils.NewAdmissionResponse(causes)
}

func ValidatingGreatDBBackupRecordSpec(field *k8sfield.Path, ns string, spec v1alpha1.GreatDBBackupRecordSpec, greatDBClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause
	causes = append(causes, ValidatingClusterName(field.Child("clusterName"), ns, spec.ClusterName, greatDBClient)...)
	causes = append(causes, ValidatingBackupRecord(field.Child("schedulers"), ns, spec.ClusterName, spec, greatDBClient)...)
	causes = append(causes, ValidatingInstanceName(field.Child("instanceName"), ns, spec.ClusterName, spec.InstanceName, greatDBClient)...)
	// causes = append(causes, ValidatingBackupRecordInstanceIsEmpty(field.Child("instanceName"), spec.InstanceName)...)
	return causes

}

func ValidatingBackupRecord(field *k8sfield.Path, ns, clusterName string, spec v1alpha1.GreatDBBackupRecordSpec, greatDBClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	cluster, err := greatDBClient.GreatdbV1alpha1().GreatDBPaxoses(ns).Get(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return causes
		}
		causes = append(causes, metav1.StatusCause{
			Type: metav1.CauseTypeUnexpectedServerResponse,
		})

		return causes
	}

	if spec.Clean != "" {
		_, err := tools.StringToDuration(spec.Clean)
		if err != nil {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueDuplicate,
				Message: fmt.Sprintf("it should be an integer+unit(m,h,d), not an %s", spec.Clean),
				Field:   field.Child("clean").String(),
			})
		}

	}

	switch spec.SelectStorage.Type {

	case v1alpha1.BackupStorageNFS:
		if cluster.Spec.Backup.NFS == nil || cluster.Spec.Backup.NFS.Path == "" || cluster.Spec.Backup.NFS.Server == "" {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: "the cluster required for backup is not configured with nfs",
				Field:   field.Child("selectStorage").Child("type").String(),
			})
		}
	case v1alpha1.BackupStorageS3:
		if spec.SelectStorage.S3 == nil || spec.SelectStorage.S3.Bucket == "" || spec.SelectStorage.S3.EndpointURL == "" || spec.SelectStorage.S3.AccessKey == "" || spec.SelectStorage.S3.SecretKey == "" {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueRequired,
				Message: "the parameters configured for s3 cannot be empty",
				Field:   field.Child("selectStorage").Child("s3").String(),
			})
		}

	}

	return causes

}

func ValidatingBackupRecordInstanceIsEmpty(field *k8sfield.Path, isnName string) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if isnName == "" {
		causes = append(causes, metav1.StatusCause{
			Type:  metav1.CauseTypeFieldValueRequired,
			Field: field.String(),
		})
	}

	return causes

}
