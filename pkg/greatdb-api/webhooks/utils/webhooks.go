package utils

import (
	"encoding/json"
	"fmt"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	"io/ioutil"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"greatdb-operator/pkg/utils/log"
)

func GetAdmissionReview(r *http.Request) (*admissionv1.AdmissionReview, error) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		return nil, fmt.Errorf("contentType=%s, expect application/json", contentType)
	}

	ar := &admissionv1.AdmissionReview{}
	err := json.Unmarshal(body, ar)
	return ar, err
}

func ToAdmissionResponseError(err error) *admissionv1.AdmissionResponse {
	log.Log.Reason(err).Error("admission generic error")

	return &admissionv1.AdmissionResponse{
		Result: &v1.Status{
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		},
	}
}

func ToAdmissionResponse(causes []v1.StatusCause) *admissionv1.AdmissionResponse {
	log.Log.Infof("rejected greatdb admission")

	globalMessage := ""
	for _, cause := range causes {
		if globalMessage == "" {
			globalMessage = cause.Message
		} else {
			globalMessage = fmt.Sprintf("%s, %s", globalMessage, cause.Message)
		}
	}

	return &admissionv1.AdmissionResponse{
		Result: &v1.Status{
			Message: globalMessage,
			Reason:  v1.StatusReasonInvalid,
			Code:    http.StatusUnprocessableEntity,
			Details: &v1.StatusDetails{
				Causes: causes,
			},
		},
	}
}

func ValidationErrorsToAdmissionResponse(errs []error) *admissionv1.AdmissionResponse {
	var causes []v1.StatusCause
	for _, e := range errs {
		causes = append(causes,
			v1.StatusCause{
				Message: e.Error(),
			},
		)
	}
	return ToAdmissionResponse(causes)
}

func ValidateRequestResource(request v1.GroupVersionResource, group string, resource string) bool {
	gvr := v1.GroupVersionResource{Group: group, Resource: resource}

	for _, version := range ApiSupportedWebhookVersions {
		gvr.Version = version
		if gvr == request {
			return true
		}
	}

	return false
}

func GetGreatDBClusterFromAdmissionReview(ar *admissionv1.AdmissionReview) (new *v1alpha1.GreatDBPaxos, old *v1alpha1.GreatDBPaxos, err error) {
	if !ValidateRequestResource(ar.Request.Resource, v1alpha1.GreatDBPaxosGroupVersionKind.Group, v1alpha1.GreatDBPaxosGroupVersionResource.Resource) {
		return nil, nil, fmt.Errorf("expect resource to be '%s'", v1alpha1.GreatDBPaxosGroupVersionResource.Resource)
	}

	raw := ar.Request.Object.Raw
	newGreatdb := v1alpha1.GreatDBPaxos{}

	err = json.Unmarshal(raw, &newGreatdb)
	if err != nil {
		return nil, nil, err
	}

	if ar.Request.Operation == admissionv1.Update {
		raw := ar.Request.OldObject.Raw
		oldCluster := v1alpha1.GreatDBPaxos{}

		err = json.Unmarshal(raw, &oldCluster)
		if err != nil {
			return nil, nil, err
		}
		return &newGreatdb, &oldCluster, nil
	}

	return &newGreatdb, nil, nil
}

func GetGreatDBBackupRecordFromAdmissionReview(ar *admissionv1.AdmissionReview) (new *v1alpha1.GreatDBBackupRecord, old *v1alpha1.GreatDBBackupRecord, err error) {
	if !ValidateRequestResource(ar.Request.Resource, v1alpha1.GreatDBBackupRecordGroupVersionKind.Group, v1alpha1.GreatDBBackupRecordGroupVersionResource.Resource) {
		return nil, nil, fmt.Errorf("expect resource to be '%s'", v1alpha1.GreatDBBackupRecordGroupVersionResource.Resource)
	}

	raw := ar.Request.Object.Raw
	newRecord := v1alpha1.GreatDBBackupRecord{}

	err = json.Unmarshal(raw, &newRecord)
	if err != nil {
		return nil, nil, err
	}

	if ar.Request.Operation == admissionv1.Update {
		raw := ar.Request.OldObject.Raw
		oldRecord := v1alpha1.GreatDBBackupRecord{}

		err = json.Unmarshal(raw, &oldRecord)
		if err != nil {
			return nil, nil, err
		}
		return &newRecord, &oldRecord, nil
	}

	return &newRecord, nil, nil
}

func GetGreatDBBackupScheduleFromAdmissionReview(ar *admissionv1.AdmissionReview) (new *v1alpha1.GreatDBBackupSchedule, old *v1alpha1.GreatDBBackupSchedule, err error) {
	if !ValidateRequestResource(ar.Request.Resource, v1alpha1.GreatDBBackupScheduleGroupVersionKind.Group, v1alpha1.GreatDBBackupScheduleGroupVersionResource.Resource) {
		return nil, nil, fmt.Errorf("expect resource to be '%s'", v1alpha1.GreatDBBackupScheduleGroupVersionResource.Resource)
	}

	raw := ar.Request.Object.Raw
	newSchedule := v1alpha1.GreatDBBackupSchedule{}

	err = json.Unmarshal(raw, &newSchedule)
	if err != nil {
		return nil, nil, err
	}

	if ar.Request.Operation == admissionv1.Update {
		raw := ar.Request.OldObject.Raw
		oldSchedule := v1alpha1.GreatDBBackupSchedule{}

		err = json.Unmarshal(raw, &oldSchedule)
		if err != nil {
			return nil, nil, err
		}
		return &newSchedule, &oldSchedule, nil
	}

	return &newSchedule, nil, nil
}
