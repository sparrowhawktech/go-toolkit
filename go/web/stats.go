package web

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"sparrowhawktech/toolkit/util"
)

type statsCounters struct {
	InCount         int64
	OutCount        int64
	TotalDuration   int64
	AverageDuration int64
}

func (o *statsCounters) Copy(other statsCounters) {
	o.InCount = other.InCount
	o.OutCount = other.OutCount
	o.TotalDuration = other.TotalDuration
	o.AverageDuration = other.AverageDuration
}

type pathStats struct {
	Path            *string
	IntervalCounter *statsCounters
	AccumCounters   *statsCounters
}

type webStats struct {
	mux   *sync.Mutex
	paths map[string]*pathStats
}

func (o *webStats) PushIn(path string) {
	o.mux.Lock()
	defer o.mux.Unlock()
	pathStats := o.resolvePathStats(path)
	pathStats.IntervalCounter.InCount++
	pathStats.AccumCounters.InCount++
}

func (o *webStats) PushOut(path string, duration int64) {
	o.mux.Lock()
	defer o.mux.Unlock()
	pathStats := o.resolvePathStats(path)
	pathStats.IntervalCounter.OutCount++
	pathStats.IntervalCounter.TotalDuration += duration
	pathStats.IntervalCounter.AverageDuration = int64(float64(pathStats.IntervalCounter.TotalDuration) / float64(pathStats.IntervalCounter.OutCount))
	pathStats.AccumCounters.OutCount++
	pathStats.AccumCounters.TotalDuration += duration
	pathStats.AccumCounters.AverageDuration = int64(float64(pathStats.AccumCounters.TotalDuration) / float64(pathStats.AccumCounters.OutCount))
}

func (o *webStats) resolvePathStats(path string) *pathStats {
	stats, ok := o.paths[path]
	if !ok {
		stats = &pathStats{
			Path: &path,
			IntervalCounter: &statsCounters{
				InCount:         0,
				OutCount:        0,
				AverageDuration: 0,
			},
			AccumCounters: &statsCounters{
				InCount:         0,
				OutCount:        0,
				AverageDuration: 0,
			},
		}
		o.paths[path] = stats
	}
	return stats
}

func (o *webStats) CheckPoint() []pathStats {
	o.mux.Lock()
	defer o.mux.Unlock()
	result := make([]pathStats, len(o.paths))
	i := 0
	for _, v := range o.paths {
		clone := pathStats{
			Path:            v.Path,
			IntervalCounter: &statsCounters{},
			AccumCounters:   &statsCounters{},
		}
		clone.IntervalCounter.Copy(*v.IntervalCounter)
		clone.AccumCounters.Copy(*v.AccumCounters)
		result[i] = clone
		v.IntervalCounter.InCount = 0
		v.IntervalCounter.OutCount = 0
		v.IntervalCounter.TotalDuration = 0
		v.IntervalCounter.AverageDuration = 0
		i++
	}
	return result
}

func (o *webStats) Start() {
	util.Log("web").Println("Web services metering activated")
	ticker := time.NewTicker(time.Minute)
	go func() {
		for range ticker.C {
			o.report()
		}
	}()
}

func (o *webStats) report() {
	defer func() {
		if r := recover(); r != nil {
			util.ProcessError(r)
		}
	}()
	snapshot := o.CheckPoint()
	sort.Slice(snapshot, func(i, j int) bool {
		return strings.Compare(*snapshot[i].Path, *snapshot[j].Path) > 0
	})
	buffer := bytes.Buffer{}
	buffer.WriteString(fmt.Sprintf("%-60s%15s%15s%15s%15s%15s%15s%15s%15s\r\n",
		"Path", "In", "Out", "Sum ms", "Avg ms", "Accum. In", "Out", "Sum ms", "Avg ms"))
	for _, v := range snapshot {
		buffer.WriteString(fmt.Sprintf("%-60s%15d%15d%15d%15d%15d%15d%15d%15d\r\n",
			*v.Path,
			v.IntervalCounter.InCount, v.IntervalCounter.OutCount, v.IntervalCounter.TotalDuration, v.IntervalCounter.AverageDuration,
			v.AccumCounters.InCount, v.AccumCounters.OutCount, v.AccumCounters.TotalDuration, v.AccumCounters.AverageDuration))
	}
	util.Log("web").Printf("Web services stats:\r\n%s", buffer.String())
}

func newStats() *webStats {
	return &webStats{
		mux:   &sync.Mutex{},
		paths: make(map[string]*pathStats),
	}
}

var stats = newStats()

func InitWebStats() {
	stats.Start()
}
