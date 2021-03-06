package statistics

import (
	"sync"
	"time"
)

type (
	SampleCollector struct {
		hotBuf  []uint64
		bufLock sync.Mutex

		curAvg         float64
		count          float64
		bufPurgeExpiry time.Duration
		bufPurgeTs     time.Time
	}
)

var (
	// DefaultHotBuffer, default size of hot buffer
	DefaultHotBuffer = 300 // todo investigate good value
	// DefaultPurgeExpiry, default time until hot buffer will be purged
	DefaultPurgeExpiry = time.Second * 5
)

// NewSampleCollector for collecting running average
func NewSampleCollector() *SampleCollector {
	return NewSampleCollectorBuf(DefaultHotBuffer)
}

// NewSampleCollector for collecting running average with custom hot buffer size
func NewSampleCollectorBuf(buf int) *SampleCollector {
	return &SampleCollector{
		hotBuf:         make([]uint64, 0, buf),
		bufPurgeExpiry: DefaultPurgeExpiry,
		bufPurgeTs:     time.Now().Add(DefaultPurgeExpiry),
	}
}

// AddSample add new sample
func (collector *SampleCollector) AddSample(sample uint64) {
	defer collector.bufLock.Unlock()
	collector.bufLock.Lock()

	if len(collector.hotBuf) == cap(collector.hotBuf) {
		collector.emptyHotBuffer()
	}

	collector.hotBuf = append(collector.hotBuf, sample)

	// async empty hot buffer independent of length of more than 5 seconds passed since last purge
	if time.Now().After(collector.bufPurgeTs) {
		go func() {
			defer collector.bufLock.Unlock()
			collector.bufLock.Lock()
			collector.emptyHotBuffer()
		}()
	}
}

// emptyHotBuffer should only be called by function holding collector.bufLock
func (collector *SampleCollector) emptyHotBuffer() {
	newCount := float64(len(collector.hotBuf))

	if newCount < 0.5 {
		// nothing in buffer
		return
	}

	var newSum float64
	for _, v := range collector.hotBuf {
		newSum += float64(v)
	}

	// first time around ?
	if collector.count < 1 {
		collector.curAvg = newSum / newCount
		collector.count = newCount
		return
	}

	// avoiding (curAvg*count + newAvg*newCount)/(count+newCount) for overflow protection, below equation equates to the same value
	collector.curAvg = collector.curAvg/(1+newCount/collector.count) + newSum/(collector.count+newCount)
	collector.count += newCount
	collector.hotBuf = collector.hotBuf[0:0]
	collector.bufPurgeTs = time.Now().Add(collector.bufPurgeExpiry)
}

// Average empties hot buffer and calculates current running average, returns average, total samples.
func (collector *SampleCollector) Average() (float64, float64) {
	defer collector.bufLock.Unlock()
	collector.bufLock.Lock()

	collector.emptyHotBuffer()

	return collector.curAvg, collector.count
}
