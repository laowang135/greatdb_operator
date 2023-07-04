package manager

import (
	"sync"
	"time"

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
	LastSyncTime    time.Time
	FrequencyRecord []time.Time
}

func (mgr *QueueManager) Add(key string, lock bool) bool {

	if lock {
		mgr.RWLock.Lock()
		defer mgr.RWLock.Unlock()
	}

	now := time.Now().Local()
	if v, ok := mgr.Resources[key]; ok {

		// Sync Interval
		if now.Sub(v.LastSyncTime) > time.Second*time.Duration(mgr.PeriodSeconds) {

			v.LastSyncTime = now

			v.FrequencyRecord = append(v.FrequencyRecord, now)
			if len(v.FrequencyRecord) >= mgr.FrequencyMinute {
				v.FrequencyRecord = v.FrequencyRecord[len(v.FrequencyRecord)-mgr.FrequencyMinute:]
			}

			mgr.Resources[key] = v
			return true
		}

		// Number of synchronizations per minute
		index := 0
		more := false
		for i, record := range v.FrequencyRecord {
			if now.Sub(record) > time.Minute {
				index = i
				more = true
				continue
			}
			break
		}
		var res []time.Time
		if more && index+1 <= len(v.FrequencyRecord) {
			res = v.FrequencyRecord[index+1:]
		}

		if len(res) < mgr.FrequencyMinute {
			v.LastSyncTime = now
			res = append(res, now)
			v.FrequencyRecord = res
			mgr.Resources[key] = v
			return true
		}

		return false

	}

	record := make([]time.Time, 0, 10)
	record = append(record, now)
	value := Rescources{
		LastSyncTime:    now,
		FrequencyRecord: record,
	}
	mgr.Resources[key] = value
	return true

}

func (mgr *QueueManager) Delete(key string) {
	mgr.RWLock.Lock()
	defer mgr.RWLock.Unlock()
	delete(mgr.Resources, key)

}

// Resources being processed
func (mgr *QueueManager) Processing(key string) bool {
	_, ok := mgr.ProcessingResources.Load(key)

	if !ok {
		mgr.ProcessingResources.Store(key, "")
		return false
	}

	return true
}

func (mgr *QueueManager) EndOfProcessing(key string) {
	mgr.ProcessingResources.Delete(key)
}

func (mgr *QueueManager) Watch(queue workqueue.RateLimitingInterface) []string {

	mgr.RWLock.Lock()
	defer mgr.RWLock.Unlock()
	keyList := make([]string, 0)
	for key := range mgr.Resources {
		if mgr.Add(key, false) {
			queue.Add(key)
			keyList = append(keyList, key)
		}
	}
	return keyList
}
