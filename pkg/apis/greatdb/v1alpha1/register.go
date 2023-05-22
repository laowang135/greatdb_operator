package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SchemeGroupVersion is group version used to register these objects.
var SchemeGroupVersion = GroupVersion

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	GreatDBClusterGroupVersionKind = schema.GroupVersionKind{Group: GroupVersion.Group, Version: GroupVersion.Version, Kind: "GreatDBPaxos"}
)

var GreatDBClusterGroupVersionResource = metav1.GroupVersionResource{
	Group:    GroupVersion.Group,
	Version:  GroupVersion.Version,
	Resource: "GreatDBPaxoses",
}
