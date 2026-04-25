package stats

type PeriodStats struct {
	TasksTotal int
	TasksOK    int
	Tokens     int64
	CostCents  int64
}

func (p PeriodStats) SuccessRate() float64 {
	if p.TasksTotal == 0 {
		return 0
	}
	return float64(p.TasksOK) / float64(p.TasksTotal)
}

type Stats struct {
	Last24h, Last7d, Last30d PeriodStats
	PendingQueue             int
}
