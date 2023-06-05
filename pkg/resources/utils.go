package resources

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
)

const (
	key = "greatk8@op-db!1l"
)

func GetGreatDBClusterOwnerReferences(name string, uuid types.UID) metav1.OwnerReference {
	isTrue := true
	return metav1.OwnerReference{
		APIVersion:         v1alpha1.SchemeGroupVersion.WithKind("GreatDBPaxos").GroupVersion().String(),
		Kind:               v1alpha1.SchemeGroupVersion.WithKind("GreatDBPaxos").Kind,
		Name:               name,
		UID:                uuid,
		Controller:         &isTrue,
		BlockOwnerDeletion: &isTrue,
	}
}

func MegerLabels(args ...map[string]string) map[string]string {
	return MegerMap(args...)
}

func MegerAnnotation(args ...map[string]string) map[string]string {
	return MegerMap(args...)
}

func MegerMap(args ...map[string]string) map[string]string {
	labels := make(map[string]string)
	for _, arg := range args {
		if arg == nil {
			continue
		}
		for key, value := range arg {
			labels[key] = value
		}
	}
	return labels
}

func GetEncodePwd(data string) string {
	return Get16MD5Encode(data)
}

func GetMD5Encode(data string) string {
	h := md5.New()
	h.Write([]byte(key))
	h.Write([]byte(data))
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))
}

func Get16MD5Encode(data string) string {
	return GetMD5Encode(data)[8:24]
}

// GetClusterUser  Return cluster user and password
func GetClusterUser(cluster *v1alpha1.GreatDBPaxos) (string, string) {

	name := cluster.Name

	return "greatdb", GetEncodePwd(name)

}

func GetInstanceFQDN(clusterName, insName, ns, clusterDomain string) string {
	// TODO Debug

	d := strings.Split(insName, "-")
	no, err := strconv.Atoi(d[len(d)-1])
	if err == nil {
		return fmt.Sprintf("172.17.120.142:%d", 30010+no)
	}

	svcName := clusterName + ComponentGreatDBSuffix

	return fmt.Sprintf("%s.%s.%s.svc.%s", insName, svcName, ns, clusterDomain)
}
