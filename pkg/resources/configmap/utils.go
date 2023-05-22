package configmap

import (
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
)

func GetNextIndex(memberList []v1alpha1.MemberCondition) int {
	index := -1

	for _, member := range memberList {
		if member.Index > index {
			index = member.Index
		}
	}
	return index + 1
}
