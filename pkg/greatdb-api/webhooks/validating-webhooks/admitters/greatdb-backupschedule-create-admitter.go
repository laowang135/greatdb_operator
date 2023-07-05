package admitters

import (
	"context"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/client/clientset/versioned"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	"greatdb-operator/pkg/utils/tools"

	"github.com/robfig/cron/v3"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/client-go/kubernetes"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"
)

type GreatDBBackupScheduleCreateAdmitter struct {
	KubeClient    kubernetes.Interface
	GreatDBClient versioned.Interface
}

func (admit GreatDBBackupScheduleCreateAdmitter) Admit(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	schedule, _, err := utils.GetGreatDBBackupScheduleFromAdmissionReview(ar)
	if err != nil {
		return utils.ToAdmissionResponseError(err)
	}
	causes := ValidatingGreatDBBackupScheduleSpec(k8sfield.NewPath("spec"), schedule.Namespace, &schedule.Spec, admit.KubeClient, admit.GreatDBClient)

	return utils.NewAdmissionResponse(causes)
}

func ValidatingGreatDBBackupScheduleSpec(field *k8sfield.Path, ns string, spec *v1alpha1.GreatDBBackupScheduleSpec, client kubernetes.Interface, greatDBClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	causes = append(causes, ValidatingClusterName(field.Child("clusterName"), ns, spec.ClusterName, greatDBClient)...)
	causes = append(causes, ValidatingSchedule(field.Child("schedulers"), ns, spec.ClusterName, spec.Schedulers, greatDBClient)...)
	causes = append(causes, ValidatingInstanceName(field.Child("instanceName"), ns, spec.ClusterName, spec.InstanceName, greatDBClient)...)

	return causes

}

func ValidatingClusterName(field *k8sfield.Path, ns, clusterName string, greatDBClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if clusterName == "" {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: "cluster name cannot be empty",
			Field:   field.String(),
		})
		return causes
	}

	_, err := greatDBClient.GreatdbV1alpha1().GreatDBPaxoses(ns).Get(context.TODO(), clusterName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: fmt.Sprintf("cluster %s does not exist in namespace %s", clusterName, ns),
				Field:   field.String(),
			})
		}
	}

	return causes

}

func ValidatingSchedule(field *k8sfield.Path, ns, clusterName string, schedulers []v1alpha1.BackupScheduler, greatDBClient versioned.Interface) []metav1.StatusCause {
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

	nameMap := make(map[string]struct{})
	for i, sch := range schedulers {
		if _, ok := nameMap[sch.Name]; ok {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueDuplicate,
				Message: fmt.Sprintf(" name %s is duplicated", sch.Name),
				Field:   field.Index(i).Child("name").String(),
			})
		} else {
			nameMap[sch.Name] = struct{}{}
		}

		if sch.Clean != "" {
			_, err := tools.StringToDuration(sch.Clean)
			if err != nil {
				causes = append(causes, metav1.StatusCause{
					Type:    metav1.CauseTypeFieldValueDuplicate,
					Message: fmt.Sprintf("it should be an integer+unit(m,h,d), not an (%s)", sch.Clean),
					Field:   field.Index(i).Child("clean").String(),
				})
			}

		}

		if sch.Schedule != "" {
			_, err := cron.ParseStandard(sch.Schedule)
			if err != nil {
				causes = append(causes, metav1.StatusCause{
					Type:    metav1.CauseTypeFieldValueDuplicate,
					Message: fmt.Sprintf("%s not the standard cron format", sch.Schedule),
					Field:   field.Index(i).Child("schedule").String(),
				})
			}
		}

		switch sch.SelectStorage.Type {

		case v1alpha1.BackupStorageNFS:
			if cluster.Spec.Backup.NFS == nil || cluster.Spec.Backup.NFS.Path == "" || cluster.Spec.Backup.NFS.Server == "" {
				causes = append(causes, metav1.StatusCause{
					Type:    metav1.CauseTypeFieldValueInvalid,
					Message: "the cluster required for backup is not configured with nfs",
					Field:   field.Index(i).Child("selectStorage").Child("type").String(),
				})
			}
		case v1alpha1.BackupStorageS3:
			if sch.SelectStorage.S3 == nil || sch.SelectStorage.S3.Bucket == "" || sch.SelectStorage.S3.EndpointURL == "" || sch.SelectStorage.S3.AccessKey == "" || sch.SelectStorage.S3.SecretKey == "" {
				causes = append(causes, metav1.StatusCause{
					Type:    metav1.CauseTypeFieldValueRequired,
					Message: "the parameters configured for s3 cannot be empty",
					Field:   field.Index(i).Child("selectStorage").Child("s3").String(),
				})
			}

		}

	}

	return causes

}

func ValidatingInstanceName(field *k8sfield.Path, ns, clusterName, instanceName string, greatDBClient versioned.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if clusterName != "" && instanceName != "" {
		cluster, err := greatDBClient.GreatdbV1alpha1().GreatDBPaxoses(ns).Get(context.TODO(), clusterName, metav1.GetOptions{})
		if err != nil {
			return causes
		}
		exist := false

		for _, mem := range cluster.Status.Member {
			if mem.Name == instanceName {
				exist = true
				break
			}
		}

		if !exist {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: fmt.Sprintf("instance %s does not belong to cluster %s", instanceName, clusterName),
				Field:   field.String(),
			})
		}

	}

	return causes

}
