package greatdbpaxos

import (
	"fmt"
	"github.com/go-sql-driver/mysql"
	"greatdb-operator/pkg/apis/greatdb/v1alpha1"
	deps "greatdb-operator/pkg/controllers/dependences"
	"greatdb-operator/pkg/resources"
	"greatdb-operator/pkg/resources/internal"
	dblog "greatdb-operator/pkg/utils/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sort"
	"strings"
)

type ClusterStatus struct {
	status           v1alpha1.ClusterDiagStatusType
	primary          InstanceStatus
	OnlineMembers    []InstanceStatus
	quorumCandidates []InstanceStatus
	gtidExecuted     map[types.UID]string
	AllInstance      []InstanceStatus
}

func (cs *ClusterStatus) getInsByPodUID(podUID types.UID) (InstanceStatus, error) {

	for _, ins := range cs.AllInstance {
		if ins.PodIns.UID == podUID {
			return ins, nil
		}
	}
	return InstanceStatus{}, fmt.Errorf("No instance found by UID: %s", podUID)
}

func (cs *ClusterStatus) selectPodWithMostGtids() types.UID {
	gtids := cs.gtidExecuted
	podIndexes := make([]types.UID, 0, len(gtids))
	for index := range gtids {
		podIndexes = append(podIndexes, index)
	}
	sort.Slice(podIndexes, func(i, j int) bool {
		return CountGtids(gtids[podIndexes[i]]) < CountGtids(gtids[podIndexes[j]])
	})
	return podIndexes[len(podIndexes)-1]
}

func (cs *ClusterStatus) createCluster(cluster *v1alpha1.GreatDBPaxos) error {
	// Creating GR cluster
	if len(cs.AllInstance) == 0 {
		return nil
	}

	is := cs.AllInstance[0]
	dblog.Log.Infof("Creating cluster %s from pod %s...", cluster.Name, is.PodIns.Name)
	err := cs.bootCluster(cluster, is)

	// rejoin other Instance
	for _, ins := range cs.AllInstance {
		if ins.PodIns.UID == is.PodIns.UID {
			continue
		}
		err = cs.rejoinInstance(ins, cluster)
		if err != nil {
			dblog.Log.Reason(err).Errorf("Rejoin Instance Failed %s.%s", cluster.Name, is.PodIns.Name)

		}
	}

	return err
}

func (cs *ClusterStatus) rebootCluster(cluster *v1alpha1.GreatDBPaxos) error {
	// Reboot GR cluster
	seedPodUID := cs.selectPodWithMostGtids()
	is, err := cs.getInsByPodUID(seedPodUID)
	if err != nil {
		return err
	}
	dblog.Log.Infof("Rebooting cluster %s from pod %s...", cluster.Name, is.PodIns.Name)

	err = cs.bootCluster(cluster, is)
	if err != nil {
		return err
	}

	// rejoin other Instance
	for _, ins := range cs.AllInstance {
		if ins.PodIns.UID == is.PodIns.UID {
			continue
		}
		err = cs.rejoinInstance(ins, cluster)
		if err != nil {
			dblog.Log.Reason(err).Errorf("Rejoin Instance Failed %s.%s", cluster.Name, is.PodIns.Name)

		}
	}

	return err
}

func (cs *ClusterStatus) bootCluster(cluster *v1alpha1.GreatDBPaxos, is InstanceStatus) error {
	client, err := is.getInstanceConnect(cluster)
	if err != nil {
		dblog.Log.Reason(err).Error("Failed to connect to the node: %s")
		return err
	}
	defer client.Close()

	user, pwd := resources.GetClusterUser(cluster)
	startGRSql := make([]string, 0)
	startGRSql = append(startGRSql, "stop group_replication;")
	startGRSql = append(startGRSql, "set global group_replication_bootstrap_group=ON;")
	startGRSql = append(startGRSql, fmt.Sprintf("start group_replication USER='%s',PASSWORD='%s';", user, pwd))
	startGRSql = append(startGRSql, "set global group_replication_bootstrap_group=OFF;")

	for i, execSql := range startGRSql {
		err = client.Exec(execSql)
		if err != nil {
			dblog.Log.Reason(err).Errorf("Failed to execute SQL statement %s.%s", cluster.Name, is.PodIns.Name)
			if i == 1 {
				client.Exec(startGRSql[2])
			}
			return err
		}
	}

	return err
}

func (cs *ClusterStatus) forceQuorum(cluster *v1alpha1.GreatDBPaxos) error {
	// Forcing quorum of cluster

	if len(cs.quorumCandidates) == 0 {
		return fmt.Errorf("cluster %s Candidates is null", cluster.Name)
	}

	is := cs.quorumCandidates[0]
	client, err := is.getInstanceConnect(cluster)
	if err != nil {
		dblog.Log.Reason(err).Error("Failed to connect to the node: %s")
		return err
	}
	// To be on the safe side
	is.QueryMembershipInfo(client)
	if is.IsPrimary {
		dblog.Log.Infof("Force Quorum cluster %s from pod %s...", cluster.Name, is.PodIns.Name)
		err = cs.bootCluster(cluster, is)

	}
	return err
}

func (cs *ClusterStatus) destroyCluster(cluster *v1alpha1.GreatDBPaxos) {
	// Stopping GR for last cluster member
}

func (cs *ClusterStatus) joinInstance(is InstanceStatus, cluster *v1alpha1.GreatDBPaxos) error {
	// Adding instance to cluster
	// TODO: 基于clone 或单机备份 添加新节点
	// stop group_replication; set global super_read_only=0; clone INSTANCE FROM GreatSQL@172.16.16.11:3306 IDENTIFIED BY 'GreatSQL';

	err := cs.rejoinInstance(is, cluster)

	return err
}

func (cs *ClusterStatus) rejoinInstance(is InstanceStatus, cluster *v1alpha1.GreatDBPaxos) error {
	// Rejoining instance to cluster
	client, err := is.getInstanceConnect(cluster)
	if err != nil {
		return err
	}
	defer client.Close()

	user, pwd := resources.GetClusterUser(cluster)
	startGRSql := make([]string, 0)
	startGRSql = append(startGRSql, "stop group_replication;")
	startGRSql = append(startGRSql, fmt.Sprintf("start group_replication USER='%s',PASSWORD='%s';", user, pwd))

	for _, execSql := range startGRSql {
		err = client.Exec(execSql)
		if err != nil {
			dblog.Log.Reason(err).Errorf("Failed to execute SQL statement %s.%s", cluster.Name, is.PodIns.Name)
			return err
		}
	}
	return nil
}

func (cs *ClusterStatus) removeInstance(is InstanceStatus, cluster *v1alpha1.GreatDBPaxos) error {
	// Removing a cluster instance
	client, err := is.getInstanceConnect(cluster)
	if err != nil {
		return err
	}
	defer client.Close()

	err = client.Exec("stop group_replication;")
	if err != nil {
		dblog.Log.Reason(err).Errorf("Removing a cluster instance %s.%s failure", cluster.Name, is.PodIns.Name)
	}
	return err
}

func (cs *ClusterStatus) initUser(cluster *v1alpha1.GreatDBPaxos) error {

	client, err := cs.primary.getInstanceConnect(cluster)
	if err != nil || client == nil {
		dblog.Log.Reason(err).Error("failed to Connect primary")
		return err
	}
	defer client.Close()

	initUser := v1alpha1.User{}

	for _, user := range cluster.Spec.Users {
		if user.Name == "" {
			continue
		}

		initialized := false
		index := -1
		for i, init := range cluster.Status.Users {
			if init.Name == user.Name && init.Password == user.Password && init.Perm == user.Perm {
				if init.Reason == "" {
					initialized = true
				} else {
					index = i
				}
				break
			}
		}
		if initialized {
			continue
		}

		sqlList := make([]string, 2)

		sqlList = append(sqlList, "alter user '"+user.Name+"'@'%' identified with mysql_native_password by "+fmt.Sprintf("'%s';", user.Password))

		if user.Perm != "" {
			sqlList = append(sqlList, user.Perm)

		}

		reason := ""
		for _, sql := range sqlList {
			err := client.Exec(sql)
			if err != nil {
				dblog.Log.Reason(err).Errorf("Failed to initialize user")
				reason = err.Error()
			}
		}
		initUser.Name = user.Name
		initUser.Password = user.Password
		initUser.Perm = user.Perm
		initUser.Reason = reason

		if index != -1 {
			cluster.Status.Users[index] = initUser
		} else {
			cluster.Status.Users = append(cluster.Status.Users, user)
		}
	}

	return nil

}

func (cs *ClusterStatus) checkInstanceContainersIsReady() bool {
	if cs.AllInstance == nil {
		return false
	}
	for _, ins := range cs.AllInstance {
		if !ins.checkContainersReady() {
			return false
		}
	}
	return true
}

func (cs *ClusterStatus) publishInstanceStatus(cluster *v1alpha1.GreatDBPaxos) {

	for i, member := range cluster.Status.Member {
		for _, ins := range cs.AllInstance {
			if ins.PodIns.Name != member.Name {
				continue
			}
			pod := ins.PodIns
			for _, cond := range pod.Status.Conditions {
				if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
					cluster.Status.Member[i].Type = v1alpha1.MemberStatusFree
					if member.Address != "" {
						break
					}
					svcName := cluster.Name + resources.ComponentGreatDBSuffix

					cluster.Status.Member[i].Address = fmt.Sprintf("%s.%s.%s.svc.%s", member.Name, svcName, cluster.Namespace, cluster.Spec.ClusterDomain)
					// TODO Debug
					// cluster.Status.Member[i].Address = resources.GetInstanceFQDN(cluster.Name, member.Name, cluster.Namespace, cluster.Spec.ClusterDomain)
					break
				}
			}
			if NeedPause(cluster, member) {
				cluster.Status.Member[i].Type = v1alpha1.MemberStatusPause
			}
			now := metav1.Now()
			cluster.Status.LastProbeTime = now
			cluster.Status.Member[i].Role = v1alpha1.MemberRoleType(ins.Role).Parse()
			cluster.Status.Member[i].LastUpdateTime = now
			cluster.Status.Member[i].LastTransitionTime = now
			cluster.Status.Member[i].Version = ins.MemberVersion
			if ins.State == v1alpha1.MemberStatusOnline {
				cluster.Status.Member[i].JoinCluster = true
			}
		}
	}
}

func (cs *ClusterStatus) publishStatus(diag ClusterStatus, cluster *v1alpha1.GreatDBPaxos) {
	// 同步集群状态
	clusterStatus := v1alpha1.ClusterStatusFailed
	switch diag.status {
	case v1alpha1.ClusterDiagStatusInitializing:
		cluster.Status.Port = cluster.Spec.Port
		cluster.Status.Instances = cluster.Spec.Instances
		cluster.Status.TargetInstances = cluster.Spec.Instances
		cluster.Status.CurrentInstances = cluster.Spec.Instances
		cluster.Status.Version = cluster.Spec.Version
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosDeployDB, "")
		clusterStatus = v1alpha1.ClusterStatusInitializing
	case v1alpha1.ClusterDiagStatusFinalizing:
		UpdateClusterStatusCondition(cluster, v1alpha1.GreatDBPaxosTerminating, "")
		clusterStatus = v1alpha1.ClusterStatusFinalizing
	case v1alpha1.ClusterDiagStatusPending, v1alpha1.ClusterDiagStatusFailed:
		clusterStatus = v1alpha1.ClusterStatusType(diag.status)
	case v1alpha1.ClusterDiagStatusOnline, v1alpha1.ClusterDiagStatusOnlinePartial, v1alpha1.ClusterDiagStatusOnlineUncertain:
		clusterStatus = v1alpha1.ClusterStatusOnline
		if cluster.Status.Version != diag.primary.MemberVersion {
			cluster.Status.Version = diag.primary.MemberVersion
		}
	case v1alpha1.ClusterDiagStatusOffline, v1alpha1.ClusterDiagStatusNoQuorum:
		clusterStatus = v1alpha1.ClusterStatusOffline
	}

	if cluster.Status.Status == v1alpha1.ClusterStatusInitializing && clusterStatus != v1alpha1.ClusterStatusOnline {
		clusterStatus = v1alpha1.ClusterStatusInitializing
	}
	if cluster.Status.DiagStatus != v1alpha1.ClusterDiagStatusPending {
		if cluster.Status.Status != clusterStatus {
			cluster.Status.Status = clusterStatus
		}
		cluster.Status.DiagStatus = diag.status
	}

	cluster.Status.ReadyInstances = int32(len(diag.OnlineMembers))
	cluster.Status.AvailableReplicas = int32(len(diag.AllInstance))

	if cluster.Status.Conditions == nil {
		cluster.Status.Conditions = make([]v1alpha1.GreatDBPaxosConditions, 0)
	}

	cs.publishInstanceStatus(cluster)
}

func (diagnostic *ClusterStatus) repairCluster(cluster *v1alpha1.GreatDBPaxos) {
	// Restore cluster to an ONLINE state

	switch diagnostic.status {
	case v1alpha1.ClusterDiagStatusOnline, v1alpha1.ClusterDiagStatusFinalizing:
		return
	case v1alpha1.ClusterDiagStatusPending, v1alpha1.ClusterDiagStatusInitializing:
		// Nothing to do
		return
	case v1alpha1.ClusterDiagStatusOnlinePartial:
		for _, ins := range diagnostic.AllInstance {
			cand := DiagnoseClusterCandidate(cluster, ins.PodIns)
			if cand.state == v1alpha1.CandidateDiagStatusRejoinable {
				err := diagnostic.rejoinInstance(ins, cluster)
				if err != nil {
					dblog.Log.Reason(err).Errorf("Rejoin Instance Failed %s.%s", cluster.Name, ins.PodIns.Name)
				}
			} else if cand.state == v1alpha1.CandidateDiagStatusJoinable {
				err := diagnostic.joinInstance(ins, cluster)
				if err != nil {
					dblog.Log.Reason(err).Errorf("Join Instance Failed %s.%s", cluster.Name, ins.PodIns.Name)
				}
			}
		}
		return
	case v1alpha1.ClusterDiagStatusOnlineUncertain:
		return
	case v1alpha1.ClusterDiagStatusOffline:
		//  Reboot cluster if all pods are reachable
		err := diagnostic.rebootCluster(cluster)
		if err != nil {
			dblog.Log.Errorf("Rebooting cluster (%s) error: %s", cluster.Name, err)
		}
		return
	case v1alpha1.ClusterDiagStatusOfflineUncertain:
		return
	case v1alpha1.ClusterDiagStatusNoQuorum:
		err := diagnostic.forceQuorum(cluster)
		if err != nil {
			dblog.Log.Errorf("ForceQuorum cluster (%s) error: %s", cluster.Name, err)
		}
		return
	case v1alpha1.ClusterDiagStatusNoQuorumUncertain:
		return
	case v1alpha1.ClusterDiagStatusSplitBrain:
		return
	case v1alpha1.ClusterDiagStatusSplitBrainUncertain:
		return
	case v1alpha1.ClusterDiagStatusUnknown:
		return
	case v1alpha1.ClusterDiagStatusInvalid:
		return
	}
}

func GetInstanceConnectToPrimary(cluster *v1alpha1.GreatDBPaxos) (internal.DBClientinterface, error) {
	for _, member := range cluster.Status.Member {
		if member.Role == v1alpha1.MemberRolePrimary {
			clientPrimary := internal.NewDBClient()
			user, pwd := resources.GetClusterUser(cluster)
			host := resources.GetInstanceFQDN(cluster.Name, member.Name, cluster.Namespace, cluster.Spec.ClusterDomain)
			port := int(cluster.Spec.Port)
			err := clientPrimary.Connect(user, pwd, host, port, "mysql")
			if err != nil {
				return nil, err
			}
			return clientPrimary, nil
		}
	}
	return nil, fmt.Errorf("Not obtained primary client")
}

func findGroupPartitions(onlineMemberStatuses map[string]InstanceStatus, onlineMemberAddress []string) ([][]InstanceStatus, [][]InstanceStatus) {
	// List of group partitions that have quorum and can execute transactions.
	// If there's more than 1, then there's a split-brain. If there's none, then we have no availability.
	activePartitions := make([][]InstanceStatus, 0)

	// List of group partitions that have no quorum and can't execute transactions.
	blockedPartitions := make([][]InstanceStatus, 0)
	noPrimaryActivePartitions := make([][]InstanceStatus, 0)
	for address, instance := range onlineMemberStatuses {
		if instance.InQuorum {
			online_peers := make([]string, 0)
			for peer, state := range instance.Peers {
				if state == string(v1alpha1.MemberStatusOnline) || state == string(v1alpha1.MemberStatusRecovering) {
					online_peers = append(online_peers, peer)
				}
			}
			missing := SubtractSlices(online_peers, onlineMemberAddress)
			if len(missing) > 0 {
				dblog.Log.Errorf("Cluster status results inconsistent: Group view of %s has %s but these are not ONLINE: %s", address, online_peers, missing)
				return activePartitions, blockedPartitions
			}

			part := make([]InstanceStatus, 0)
			for peer, state := range instance.Peers {
				if state == string(v1alpha1.MemberStatusOnline) || state == string(v1alpha1.MemberStatusRecovering) {
					part = append(part, onlineMemberStatuses[peer])
				}
			}
			if instance.IsPrimary {
				activePartitions = append(activePartitions, part)
			} else {
				noPrimaryActivePartitions = append(noPrimaryActivePartitions, part)
			}
		}
	}

	if len(activePartitions) == 0 && len(noPrimaryActivePartitions) > 0 {
		dblog.Log.Error("Cluster has quorum but no PRIMARY: it's possible" +
			" for a group with quorum to not have a PRIMARY for a short time if the PRIMARY is removed from the group")
		return activePartitions, blockedPartitions
	}

	activePartitionWith := func(address string) []InstanceStatus {
		for _, part := range activePartitions {
			for _, p := range part {
				if p.MemberHost == address {
					return part
				}
			}
		}
		return nil
	}

	for address, instance := range onlineMemberStatuses {
		if !instance.InQuorum {
			partRes := activePartitionWith(address)
			if partRes == nil {
				dblog.Log.Errorf("Inconsistent group view, %s not expected to be in %s", address, partRes)
				return activePartitions, blockedPartitions
			}

			part := make([]InstanceStatus, 0)
			for peer, state := range instance.Peers {
				if state != string(v1alpha1.MemberStatusUnmanaged) {
					part = append(part, onlineMemberStatuses[peer])
				}
			}
			if len(blockedPartitions) == 0 {
				blockedPartitions = append(blockedPartitions, part)
			}
			for _, v := range blockedPartitions {
				if !reflect.DeepEqual(part, v) {
					blockedPartitions = append(blockedPartitions, part)
				}
			}

		}
	}

	sortPartitions := func(s [][]InstanceStatus) {
		// Sort partitions largest to smallest
		sort.Slice(s, func(i, j int) bool {
			return len(s[i]) > len(s[j])
		})
	}
	sortPartitions(activePartitions)
	sortPartitions(blockedPartitions)
	return activePartitions, blockedPartitions
}

type InstanceStatus struct {
	PodIns               *corev1.Pod
	ConnectError         string
	ConnectErrorCode     uint16
	IsPrimary            bool
	InQuorum             bool
	Peers                map[string]string
	gtidUnion            string
	MemberHost           string                       `json:member_host,omitempty`
	MemberPort           string                       `json:member_port,omitempty`
	ViewID               string                       `json:view_id,omitempty`
	Role                 string                       `json:member_role,omitempty`
	State                v1alpha1.MemberConditionType `json:member_state,omitempty`
	MemberId             string                       `json:member_id,omitempty`
	MemberVersion        string                       `json:member_version,omitempty`
	MemberCount          int32                        `json:member_count,omitempty`
	ReachableMemberCount int32                        `json:reachable_member_count,omitempty`
}

func (is *InstanceStatus) getInstanceConnect(cluster *v1alpha1.GreatDBPaxos) (internal.DBClientinterface, error) {
	client := internal.NewDBClient()
	user, pwd := resources.GetClusterUser(cluster)
	host := resources.GetInstanceFQDN(cluster.Name, is.PodIns.Name, cluster.Namespace, cluster.Spec.ClusterDomain)
	port := int(cluster.Spec.Port)
	err := client.Connect(user, pwd, host, port, "mysql")

	errorCodePtr, errorMessagePtr := client.GetError()
	if errorCodePtr != nil {
		is.ConnectErrorCode = *errorCodePtr
	}
	if errorMessagePtr != nil {
		is.ConnectError = *errorMessagePtr
	}
	if err != nil {
		dblog.Log.Reason(err).Errorf("Failed to connect to the cluster %s node (%s)", cluster.Name, is.PodIns.Name)
		return client, err
	}
	return client, err
}

func (is *InstanceStatus) QueryMembershipInfo(client internal.DBClientinterface) {
	err := client.QueryRow(QueryMemberInfo, &is.MemberId, &is.MemberHost, &is.MemberPort, &is.Role, &is.State, &is.ViewID,
		&is.MemberVersion, &is.MemberCount, &is.ReachableMemberCount)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			is.State = v1alpha1.MemberStatusOffline
			return
		} else {
			dblog.Log.Reason(err).Error("failed to query member status")
			is.State = v1alpha1.MemberStatusUnknown
			return
		}
	}
	if is.Role == "PRIMARY" {
		is.IsPrimary = true
		is.InQuorum = true
	} else if is.Role == "SECONDARY" && is.State == v1alpha1.MemberStatusOnline {
		is.InQuorum = true
	} else {
		is.InQuorum = false
	}

	memberList := make([]resources.PaxosMember, 0)
	err = client.Query(QueryClusterMemberStatus, &memberList, []string{})
	if err != nil {
		dblog.Log.Reason(err).Errorf("failed to query cluster status")
		return
	}

	for _, status := range memberList {
		if status.Host != "" {
			is.Peers[status.Host] = status.State
		}
	}

	return
}

func (is *InstanceStatus) checkErrantGtids(cluster *v1alpha1.GreatDBPaxos) (string, error) {
	var errants string

	client, err := is.getInstanceConnect(cluster)
	if err != nil {
		dblog.Log.Reason(err).Error("Failed to connect to the node: %s")
		return errants, err
	}
	defer client.Close()
	currentGtidUnion := is.getGtidUnion(client, cluster)

	if currentGtidUnion != "" {
		clientPrimary, connErr := GetInstanceConnectToPrimary(cluster)
		if connErr != nil {
			return errants, connErr
		}
		primaryGtidUnion := is.getGtidUnion(clientPrimary, cluster)
		gtidCompareSql := fmt.Sprintf("SELECT GTID_SUBTRACT('%s','%s');", currentGtidUnion, primaryGtidUnion)

		err = clientPrimary.QueryRow(gtidCompareSql, &errants)
		if err != nil {
			dblog.Log.Reason(err).Errorf("failed to exec sql: %s", gtidCompareSql)
			return errants, err
		}
	}
	return errants, nil
}

func (is *InstanceStatus) setGtidUnion(client internal.DBClientinterface, cluster *v1alpha1.GreatDBPaxos) {
	is.gtidUnion = is.getGtidUnion(client, cluster)
}

func (is *InstanceStatus) getGtidUnion(client internal.DBClientinterface, cluster *v1alpha1.GreatDBPaxos) string {

	gtidExecutedSql := "SELECT @@GLOBAL.GTID_EXECUTED"
	var gtidExecuted string
	err := client.QueryRow(gtidExecutedSql, &gtidExecuted)
	if err != nil {
		dblog.Log.Reason(err).Errorf("failed to exec sql: %s", gtidExecutedSql)
		return ""
	}
	fmt.Println("Gtid Executed:", gtidExecuted)

	receiveGtidSql := "select received_transaction_set from performance_schema.replication_connection_status where channel_name=\"group_replication_applier\";"
	var receiveGtid string
	err = client.QueryRow(receiveGtidSql, &receiveGtid)
	if err != nil {
		dblog.Log.Reason(err).Errorf("failed to exec sql: %s", receiveGtidSql)
		return ""
	}
	fmt.Println("Gtid Receive:", receiveGtid)

	gtidUnionSql := fmt.Sprintf("select gtid_union(\"%s\", \"%s\")", gtidExecuted, receiveGtid)
	var gtidUnion string
	err = client.QueryRow(gtidUnionSql, &gtidUnion)
	if e, ok := err.(*mysql.MySQLError); ok {
		if e.Number == internal.ER_SP_DOES_NOT_EXIST {
			clientPrimary, connErr := GetInstanceConnectToPrimary(cluster)
			if connErr != nil {
				dblog.Log.Reason(connErr).Error("Failed to connect to the Primary node: %s")
				return ""
			}

			functionDefs := []string{
				"CREATE FUNCTION GTID_NORMALIZE(g LONGTEXT) RETURNS LONGTEXT RETURN GTID_SUBTRACT(g, '')",
				"CREATE FUNCTION GTID_UNION(gtid_set_1 LONGTEXT, gtid_set_2 LONGTEXT) RETURNS LONGTEXT RETURN GTID_NORMALIZE(CONCAT(gtid_set_1, ',', gtid_set_2))",
			}
			// Execute the CREATE FUNCTION statements
			for _, functionDef := range functionDefs {
				err = clientPrimary.Exec(functionDef)
				if err != nil {
					if mysqlErr, ok := err.(*mysql.MySQLError); ok {
						if mysqlErr.Number != 1304 {
							fmt.Printf("CREATE FUNCTION ERROR %d: %s", mysqlErr.Number, mysqlErr.Message)
							return ""
						}
					}
				}
			}
			// retry
			err = client.QueryRow(gtidUnionSql, &gtidUnion)
		}
	}

	if err != nil {
		dblog.Log.Reason(err).Errorf("failed to exec sql: %s", gtidUnionSql)
	} else {
		fmt.Println("Gtid Union:", gtidUnion)
	}
	return gtidUnion
}

func (is *InstanceStatus) checkContainersReady() bool {
	return is.checkCondition(corev1.ContainersReady)
}

func (is *InstanceStatus) checkCondition(condType corev1.PodConditionType) bool {
	if is.PodIns.Status.Conditions != nil {
		for _, cond := range is.PodIns.Status.Conditions {
			if cond.Type == condType {
				return cond.Status == "True"
			}
		}
	}
	return false
}

func (is *InstanceStatus) updateMembershipStatus() {

}

func diagnoseInstance(cluster *v1alpha1.GreatDBPaxos, pod *corev1.Pod) (InstanceStatus, error) {
	is := InstanceStatus{
		PodIns: pod,
		Peers:  make(map[string]string),
	}
	client, err := is.getInstanceConnect(cluster)

	if err != nil {
		if internal.CR_MAX_ERROR >= is.ConnectErrorCode && is.ConnectErrorCode >= internal.CR_MIN_ERROR {
			// client side errors mean we can't connect to the server, but the
			// problem could be in the client or network and not the server
			dblog.Log.Warningf("%s: pod.phase=%s, deleting=%s", is.PodIns.Spec.Hostname, is.PodIns.Status.Phase, !is.PodIns.DeletionTimestamp.IsZero())

			if is.PodIns.Status.Phase != "Running" || is.checkContainersReady() || is.PodIns.DeletionTimestamp.IsZero() {
				// not ONLINE for sure if the Pod is not running
				is.State = v1alpha1.MemberStatusOffline
			}
		} else if strings.Contains(err.Error(), "reason: context deadline exceeded") {
			if is.PodIns.Status.Phase == "Running" && is.checkContainersReady() && !is.PodIns.DeletionTimestamp.IsZero() {
				is.State = v1alpha1.MemberStatusError
			} else {
				is.State = v1alpha1.MemberStatusUnknown
			}

		} else {
			if internal.CheckFatalConnect(is.ConnectErrorCode) {
				dblog.Log.Errorf("Unexpected error connecting to MySQL. This error is not expected and may "+
					"indicate a bug or corrupted cluster: error=%s target=%s", err, is.PodIns.Spec.Hostname)
				return is, err
			}
		}

		return is, err
	}
	defer client.Close()
	is.QueryMembershipInfo(client)
	is.setGtidUnion(client, cluster)
	return is, err
}

type CandidateStatus struct {
	state      v1alpha1.CandidateDiagStatus
	badGtidSet string
}

func DiagnoseClusterCandidate(cluster *v1alpha1.GreatDBPaxos, pod *corev1.Pod) CandidateStatus {
	// Check status of an instance that's about to be added to the cluster or
	// rejoin it, relative to the given cluster. Also checks whether the instance can join it.

	status := CandidateStatus{}
	is, err := diagnoseInstance(cluster, pod)

	if err != nil {
		dblog.Log.Reason(err).Error("failed Instance")
	}

	if is.State == v1alpha1.MemberStatusUnknown {
		status.state = v1alpha1.CandidateDiagStatusUnreachable
	} else if is.State == v1alpha1.MemberStatusOnline || is.State == v1alpha1.MemberStatusRecovering {
		status.state = v1alpha1.CandidateDiagStatusMember
	} else if is.State == v1alpha1.MemberStatusUnmanaged {
		//check_errant_gtids

		status.badGtidSet, err = is.checkErrantGtids(cluster)
		if status.badGtidSet == "" {
			status.state = v1alpha1.CandidateDiagStatusJoinable
		} else {
			dblog.Log.Warningf("%s has errant transactions relative to the cluster: errant_gtids={%s}", pod.Name, status.badGtidSet)
			status.state = v1alpha1.CandidateDiagStatusUnsuitable
		}

	} else if is.State == v1alpha1.MemberStatusOffline || is.State == v1alpha1.MemberStatusError {
		fatalError := ""
		if is.State == v1alpha1.MemberStatusError {
			// TODO: check for fatal GR errors
			fatalError = ""
		} else {
			fatalError = ""
		}

		status.badGtidSet, err = is.checkErrantGtids(cluster)
		if status.badGtidSet != "" {
			dblog.Log.Warningf("%s has errant transactions relative to the cluster: errant_gtids={%s}", pod.Name, status.badGtidSet)
		}

		// Check whether you have joined a cluster in the past
		var joinCluster bool
		for _, mem := range cluster.Status.Member {
			if pod.Name == mem.Name && mem.JoinCluster {
				joinCluster = true
			}
		}

		if joinCluster {
			if status.badGtidSet == "" && fatalError == "" {
				status.state = v1alpha1.CandidateDiagStatusRejoinable
			} else {
				status.state = v1alpha1.CandidateDiagStatusBroken
			}
		} else {
			if status.badGtidSet == "" && fatalError == "" {
				status.state = v1alpha1.CandidateDiagStatusJoinable
			} else {
				status.state = v1alpha1.CandidateDiagStatusUnsuitable
			}
		}
	} else {
		dblog.Log.Errorf("Unexpected pod state pod=%s  status=%s", pod.Name, is.State)
	}

	return status
}

func DiagnoseCluster(cluster *v1alpha1.GreatDBPaxos, lister *deps.Listers) ClusterStatus {
	clusterStatus := ClusterStatus{}

	if cluster.Status.DiagStatus == "" && cluster.DeletionTimestamp.IsZero() {
		clusterStatus.status = v1alpha1.ClusterDiagStatusInitializing
		return clusterStatus
	}

	if !cluster.DeletionTimestamp.IsZero() {
		clusterStatus.status = v1alpha1.ClusterDiagStatusFinalizing
		return clusterStatus
	}

	allMemberPods := make([]v1alpha1.MemberCondition, 0)
	onlinePods := make([]v1alpha1.MemberCondition, 0)
	offlinePods := make([]v1alpha1.MemberCondition, 0)
	unsurePods := make([]v1alpha1.MemberCondition, 0)
	gtidExecuted := make(map[types.UID]string)

	onlineMemberStatuses := make(map[string]InstanceStatus)
	onlineMemberAddress := make([]string, 0)
	clusterStatus.AllInstance = make([]InstanceStatus, 0)
	for _, member := range cluster.Status.Member {
		pod, err := lister.PodLister.Pods(cluster.Namespace).Get(member.Name)
		if err != nil {
			dblog.Log.Error(err.Error())
			continue
		}
		instanceStatus, err := diagnoseInstance(cluster, pod)
		clusterStatus.AllInstance = append(clusterStatus.AllInstance, instanceStatus)
		if err != nil {
			dblog.Log.Error(err.Error())
			continue
		}
		gtidExecuted[pod.UID] = instanceStatus.gtidUnion
		allMemberPods = append(allMemberPods, member)
		onlineMemberAddress = append(onlineMemberAddress, member.Address)
		if instanceStatus.State == v1alpha1.MemberStatusUnknown {
			unsurePods = append(unsurePods, member)
		} else if instanceStatus.State == v1alpha1.MemberStatusOffline || instanceStatus.State == v1alpha1.MemberStatusError || instanceStatus.State == v1alpha1.MemberStatusUnknown {
			offlinePods = append(offlinePods, member)
		} else if instanceStatus.State == v1alpha1.MemberStatusOnline || instanceStatus.State == v1alpha1.MemberStatusRecovering {
			onlinePods = append(onlinePods, member)
			onlineMemberStatuses[member.Address] = instanceStatus
		} else {
			dblog.Log.Errorf("instance status invalid: %s", instanceStatus.State)
		}
	}
	clusterStatus.gtidExecuted = gtidExecuted

	if len(onlinePods) > 0 {
		activePartitions, blockedPartitions := findGroupPartitions(onlineMemberStatuses, onlineMemberAddress)
		dblog.Log.Infof("active_partitions=%s  blocked_partitions=%s", activePartitions, blockedPartitions)
		if len(activePartitions) == 0 {
			if len(unsurePods) > 0 {
				clusterStatus.status = v1alpha1.ClusterDiagStatusNoQuorumUncertain
			} else {
				clusterStatus.status = v1alpha1.ClusterDiagStatusNoQuorum
			}
			if len(blockedPartitions) > 0 {
				clusterStatus.quorumCandidates = blockedPartitions[0]
			}
		} else if len(activePartitions) == 1 {
			if len(unsurePods) > 0 {
				clusterStatus.status = v1alpha1.ClusterDiagStatusOnlineUncertain
			} else if len(offlinePods) > 0 {
				clusterStatus.status = v1alpha1.ClusterDiagStatusOnlinePartial
			} else {
				clusterStatus.status = v1alpha1.ClusterDiagStatusOnline
			}
			clusterStatus.OnlineMembers = activePartitions[0]

			for _, p := range activePartitions[0] {
				if p.IsPrimary {
					clusterStatus.primary = p
				}
			}

		} else {
			// split-brain
			if len(unsurePods) > 0 {
				clusterStatus.status = v1alpha1.ClusterDiagStatusSplitBrainUncertain
			} else {
				clusterStatus.status = v1alpha1.ClusterDiagStatusSplitBrain
			}
			clusterStatus.OnlineMembers = make([]InstanceStatus, 0)
			for _, part := range activePartitions {
				for _, p := range part {
					clusterStatus.OnlineMembers = append(clusterStatus.OnlineMembers, p)
				}
			}
		}
	} else {
		if !cluster.DeletionTimestamp.IsZero() {
			clusterStatus.status = v1alpha1.ClusterDiagStatusFinalizing
		} else {
			if len(offlinePods) > 0 {
				if len(unsurePods) > 0 {
					clusterStatus.status = v1alpha1.ClusterDiagStatusOfflineUncertain
				} else {
					clusterStatus.status = v1alpha1.ClusterDiagStatusOffline
				}
			} else {
				clusterStatus.status = v1alpha1.ClusterDiagStatusUnknown
			}
		}
	}
	dblog.Log.Infof("Cluster %s  status=%s", cluster.Name, clusterStatus.status)

	return clusterStatus
}

func (great GreatDBManager) probeMemberStatus(cluster *v1alpha1.GreatDBPaxos, pod *corev1.Pod) {
	instanceStatus, err := diagnoseInstance(cluster, pod)
	if err != nil {
		dblog.Log.Reason(err).Error("failed Instance")
	}
	if pod.DeletionTimestamp.IsZero() {
		instanceStatus.updateMembershipStatus()
	}
}

func (great GreatDBManager) probeStatusIfNeeded(cluster *v1alpha1.GreatDBPaxos) (diag ClusterStatus) {
	if updateStatusCheck(cluster) {
		diag = great.ProbeStatus(cluster)
		return diag
	}
	if cluster.Status.DiagStatus == v1alpha1.ClusterDiagStatusUnknown {
		dblog.Log.Errorf("Cluster has unreachable members. ")
	}
	return diag
}

func (great GreatDBManager) ProbeStatus(cluster *v1alpha1.GreatDBPaxos) ClusterStatus {
	diag := DiagnoseCluster(cluster, great.Lister)
	if diag.status != v1alpha1.ClusterDiagStatusFinalizing {
		diag.publishStatus(diag, cluster)
	}

	return diag
}
