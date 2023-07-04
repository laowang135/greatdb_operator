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

type GreatDBBackupScheduleUpdateAdmitter struct {
	KubeClient    kubernetes.Interface
	GreatDBClient versioned.Interface
}

func (admit GreatDBBackupScheduleUpdateAdmitter) Admit(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	newSchedule, oldSchedule, err := utils.GetGreatDBBackupScheduleFromAdmissionReview(ar)
	if err != nil {
		return utils.ToAdmissionResponseError(err)
	}

	if !reflect.DeepEqual(newSchedule, oldSchedule) {
		causes := ValidatingGreatDBBackupScheduleUpdateSpec(k8sfield.NewPath("spec"), newSchedule, oldSchedule, admit.KubeClient, admit.GreatDBClient)
		return utils.NewAdmissionResponse(causes)
	}
	return utils.NewPassingAdmissionResponse()
}

func ValidatingGreatDBBackupScheduleUpdateSpec(field *k8sfield.Path, schedule, oldSchedule *v1alpha1.GreatDBBackupSchedule, kubeClient kubernetes.Interface, greatdbClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	causes = append(causes, ValidatingClusterNameUpdate(field.Child("clusterName"), schedule.Namespace, schedule.Spec.ClusterName, oldSchedule.Spec.ClusterName, greatdbClient)...)
	causes = append(causes, ValidatingSchedule(field.Child("schedulers"), schedule.Namespace, schedule.Spec.ClusterName, schedule.Spec.Schedulers, greatdbClient)...)
	causes = append(causes, ValidatingInstanceName(field.Child("instanceName"), schedule.Namespace, schedule.Spec.ClusterName, schedule.Spec.InstanceName, greatdbClient)...)

	return causes
}

func ValidatingClusterNameUpdate(field *k8sfield.Path, ns, clusterName, oldClusterName string, greatDBClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if clusterName != oldClusterName {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: "prohibit modification",
			Field:   field.String(),
		})
		return causes
	}

	return causes

}
