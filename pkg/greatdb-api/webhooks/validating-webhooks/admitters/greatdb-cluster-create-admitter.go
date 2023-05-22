package admitters

import (
	"context"
	"fmt"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	"greatdb-operator/pkg/utils/log"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/client-go/kubernetes"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfield "k8s.io/apimachinery/pkg/util/validation/field"
)

type GreatDBClusterCreateAdmitter struct {
	Client kubernetes.Interface
}

func (admit GreatDBClusterCreateAdmitter) Admit(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	newCluster, _, err := utils.GetGreatDBClusterFromAdmissionReview(ar)
	if err != nil {
		return utils.ToAdmissionResponseError(err)
	}
	causes := ValidatingGreatDBClusterSpec(k8sfield.NewPath("spec"), newCluster.Namespace, &newCluster.Spec, admit.Client)

	return utils.NewAdmissionResponse(causes)

}

func ValidatingGreatDBClusterSpec(field *k8sfield.Path, ns string, spec *v1alpha1.GreatDBPaxosSpec, client kubernetes.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause
	causes = append(causes, ValidatingSecretName(field.Child("secretName"), ns, spec.SecretName, client)...)
	causes = append(causes, ValidatingPriorityClassName(field.Child("priorityClassName"), spec.PriorityClassName, client)...)
	causes = append(causes, ValidatingImagePullSecrets(field.Child("imagePullSecrets"), ns, spec.ImagePullSecrets, client)...)

	return causes

}

func ValidatingSecretName(field *k8sfield.Path, ns, secretName string, client kubernetes.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if secretName == "" {
		return causes
	}

	_, err := client.CoreV1().Secrets(ns).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil && k8serrors.IsNotFound(err) {
		log.Log.Error(err.Error())
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueNotFound,
			Message: fmt.Sprintf("secret %s/%s not exist", ns, secretName),
			Field:   field.String(),
		})
	}
	return causes

}

func ValidatingPriorityClassName(field *k8sfield.Path, name string, client kubernetes.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause
	if name == "" {
		return causes
	}

	_, err := client.SchedulingV1().PriorityClasses().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil && k8serrors.IsNotFound(err) {
		log.Log.Error(err.Error())
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueNotFound,
			Message: fmt.Sprintf(" PriorityClasses %s not exist", name),
			Field:   field.String(),
		})
	}

	return causes

}

func ValidatingImagePullSecrets(field *k8sfield.Path, ns string, imagePullSecrets []corev1.LocalObjectReference, client kubernetes.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	data := make(map[string]struct{})
	for _, local := range imagePullSecrets {
		if _, ok := data[local.Name]; ok {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueDuplicate,
				Message: fmt.Sprintf("secret %s/%s repeat", ns, local.Name),
				Field:   field.String(),
			})
			return causes
		} else {
			data[local.Name] = struct{}{}
		}
	}

	for _, local := range imagePullSecrets {
		secret, err := client.CoreV1().Secrets(ns).Get(context.TODO(), local.Name, metav1.GetOptions{})
		if err != nil && k8serrors.IsNotFound(err) {
			log.Log.Error(err.Error())
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueNotFound,
				Message: fmt.Sprintf("secret %s/%s not exist", ns, local.Name),
				Field:   field.String(),
			})
			continue
		}

		if secret.Type != corev1.SecretTypeDockerConfigJson {
			causes = append(causes, metav1.StatusCause{
				Type:    metav1.CauseTypeFieldValueInvalid,
				Message: fmt.Sprintf("secret %s/%s  is not type %s", ns, local.Name, corev1.SecretTypeDockerConfigJson),
				Field:   field.String(),
			})
		}

	}

	return causes
}
