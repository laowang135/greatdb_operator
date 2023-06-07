package admitters

import (
	"context"
	"fmt"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"greatdb-operator/pkg/greatdb-api/webhooks/utils"
	"greatdb-operator/pkg/resources"
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
	causes = append(causes, ValidatingPort(field.Child("port"), ns, spec.Port)...)
	causes = append(causes, ValidatingService(field.Child("service"), spec.Service, client)...)
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

func ValidatingPort(field *k8sfield.Path, ns string, port int32) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if port == resources.GroupPort {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: fmt.Sprintf("the service port(%d) is not allowed to be the same as the group replication port(%d)", port, resources.GroupPort),
			Field:   field.String(),
		})
	}

	return causes
}

func ValidatingService(field *k8sfield.Path, service v1alpha1.ServiceType, client kubernetes.Interface) []metav1.StatusCause {
	var causes []metav1.StatusCause

	if service.Type != corev1.ServiceTypeNodePort {
		return causes
	}

	if service.ReadPort == 0 && service.WritePort == 0 {
		return causes
	}

	if service.ReadPort == service.WritePort {
		causes = append(causes, metav1.StatusCause{
			Type:    metav1.CauseTypeFieldValueInvalid,
			Message: fmt.Sprintf("Invalid value: readPort【%d], writePort【%d】: Prohibit setting to the same value", service.ReadPort, service.WritePort),
			Field:   field.String(),
		})
	}

	serviceList, err := client.CoreV1().Services(corev1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return causes
	}

	read := false
	if service.ReadPort == 0 {
		read = true
	}
	write := false
	if service.WritePort == 0 {
		write = true
	}
	for _, svc := range serviceList.Items {

		if read && write {
			break
		}
		if svc.Spec.Type != corev1.ServiceTypeNodePort {
			continue
		}

		for _, port := range svc.Spec.Ports {
			if port.NodePort == service.ReadPort {
				read = true
				causes = append(causes, metav1.StatusCause{
					Type:    metav1.CauseTypeFieldValueInvalid,
					Message: fmt.Sprintf("Invalid value: %d: provided port is already allocated", service.ReadPort),
					Field:   field.Child("readPort").String(),
				})
			}

			if port.NodePort == service.WritePort {
				write = true
				causes = append(causes, metav1.StatusCause{
					Type:    metav1.CauseTypeFieldValueInvalid,
					Message: fmt.Sprintf("Invalid value: %d: provided port is already allocated", service.WritePort),
					Field:   field.Child("writePort").String(),
				})
			}
		}
	}

	return causes
}
