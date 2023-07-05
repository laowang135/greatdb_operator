package dependences

import (
	"sync"
	"time"

	"github.com/pkg/errors"
	cron "github.com/robfig/cron/v3"
)

type CronRegistry struct {
	Client     *ClientSet
	Crons      *cron.Cron
	BackupJobs *sync.Map
}

var CronRegistryManager CronRegistry

// AddFuncWithSeconds does the same as cron.AddFunc but changes the schedule so that the function will run the exact second that this method is called.
func (r *CronRegistry) AddFuncWithSeconds(spec string, cmd func()) (cron.EntryID, error) {
	schedule, err := cron.ParseStandard(spec)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse cron schedule")
	}
	schedule.(*cron.SpecSchedule).Second = uint64(1 << time.Now().Second())
	id := r.Crons.Schedule(schedule, cron.FuncJob(cmd))
	return id, nil
}

type Schedule struct {
	ID           int
	CronSchedule string
}

func NewCronRegistry(client *ClientSet) CronRegistry {
	CronRegistryManager = CronRegistry{
		Client:     client,
		Crons:      cron.New(),
		BackupJobs: new(sync.Map),
	}

	CronRegistryManager.Crons.Start()

	return CronRegistryManager
}
