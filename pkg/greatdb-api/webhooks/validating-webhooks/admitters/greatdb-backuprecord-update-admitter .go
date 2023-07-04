package admitters

import (
	"reflect"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/client-go/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"
)

type GreatDBBackupRecordUpdateAdmitter struct {
	KubeClient    kubernetes.Interface
	GreatDBClient versioned.Interface
}

func (admit GreatDBBackupRecordUpdateAdmitter) Admit(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	newRecord, oldRecord, err := utils.GetGreatDBBackupRecordFromAdmissionReview(ar)
	if err != nil {
		return utils.ToAdmissionResponseError(err)
	}

	if !reflect.DeepEqual(newRecord, oldRecord) {
		causes := ValidatingGreatDBBackupRecordUpdateSpec(k8sfield.NewPath("spec"), newRecord, oldRecord, admit.GreatDBClient)
		return utils.NewAdmissionResponse(causes)
	}
	return utils.NewPassingAdmissionResponse()
}

func ValidatingGreatDBBackupRecordUpdateSpec(field *k8sfield.Path, record, oldRecord *v1alpha1.GreatDBBackupRecord, greatdbClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	causes = append(causes, ValidatingBackupRecord(field.Child("schedulers"), record.Namespace, record.Spec.ClusterName, record.Spec, greatdbClient)...)
	causes = append(causes, ValidatingClusterNameUpdate(field.Child("clusterName"), record.Namespace, record.Spec.ClusterName, oldRecord.Spec.ClusterName, greatdbClient)...)
	causes = append(causes, ValidatingInstanceName(field.Child("instanceName"), record.Namespace, record.Spec.ClusterName, record.Spec.InstanceName, greatdbClient)...)
	causes = append(causes, ValidatingInstanceNameUpdate(field.Child("instanceName"), record.Spec.InstanceName, oldRecord.Spec.InstanceName)...)

	return causes
}

func ValidatingInstanceNameUpdate(field *k8sfield.Path, insName, oldInsName string) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if insName != oldInsName {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: "prohibit modification",
			Field:   field.String(),
		})
		return causes
	}

	return causes

}
