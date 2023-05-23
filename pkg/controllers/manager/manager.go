package manager

import (
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
)

func NewResourcesManager(PeriodSeconds, FrequencyMinute int) *QueueManager {

	return &QueueManager{
		Resources:           make(map[string]Rescources),
		PeriodSeconds:       PeriodSeconds,
		FrequencyMinute:     FrequencyMinute,
		RWLock:              &sync.RWMutex{},
		ProcessingResources: &sync.Map{},
	}
}

type QueueManager struct {
	Resources           map[string]Rescources
	ProcessingResources *sync.Map
	PeriodSeconds       int // At least synchronize in this time
	FrequencyMinute     int // Sync up to several times per minute
	RWLock              *sync.RWMutex
}

type Rescources struct {
	LastSyncTime    metav1.Time
	FrequencyRecord []metav1.Time
}

func (mgr QueueManager) Add(key string, lock bool) bool {

	if lock {
		mgr.RWLock.Lock()
		defer mgr.RWLock.Unlock()
	}

	now := metav1.Now()
	if v, ok := mgr.Resources[key]; ok {

		// Sync Interval
		if now.Sub(v.LastSyncTime.Time) > time.Second*time.Duration(mgr.PeriodSeconds) {
			v.LastSyncTime = now
			v.FrequencyRecord = append(v.FrequencyRecord, now)
			mgr.Resources[key] = v
			return true
		}

		// Number of synchronizations per minute
		index := 0
		for i, record := range v.FrequencyRecord {
			if now.Sub(record.Time) > time.Minute {
				index = i
			}
		}
		if index+1 < len(v.FrequencyRecord) {
			index += 1
		}

		res := v.FrequencyRecord[index:]
		if len(res) < mgr.FrequencyMinute {
			v.LastSyncTime = now
			res = append(res, now)
			v.FrequencyRecord = res
			mgr.Resources[key] = v
			return true
		}
		mgr.Resources[key] = v
		return false

	}

	record := make([]metav1.Time, 0, 10)
	record = append(record, now)
	value := Rescources{
		LastSyncTime:    now,
		FrequencyRecord: record,
	}
	mgr.Resources[key] = value
	return true

}

func (mgr QueueManager) Delete(key string) {
	mgr.RWLock.Lock()
	defer mgr.RWLock.Unlock()
	delete(mgr.Resources, key)

}

// Resources being processed
func (mgr QueueManager) Processing(key string) bool {
	_, ok := mgr.ProcessingResources.Load(key)

	if !ok {
		mgr.ProcessingResources.Store(key, "")
		return false
	}

	return true
}

func (mgr QueueManager) EndOfProcessing(key string) {
	mgr.ProcessingResources.Delete(key)
}

func (mgr QueueManager) Watch(queue workqueue.RateLimitingInterface) {

	mgr.RWLock.Lock()
	defer mgr.RWLock.Unlock()
	for key := range mgr.Resources {
		if mgr.Add(key, false) {
			queue.Add(key)
		}
	}

}
