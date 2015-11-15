package elib

import (
	"math"
	"sync"
	"time"
)

var (
	// Ticks per second of event timer (and inverse).
	cyclesPerSec, secsPerCycle float64
	cyclesOnce                 sync.Once
)

func CPUTimeInit() {
	cyclesOnce.Do(func() {
		go estimateFrequency(1e-3, 1e6, 1e4)
	})
}

func CPUCyclesPerSec() float64 {
	// Wait until estimateFrequency is done.
	for cyclesPerSec == 0 {
	}
	return cyclesPerSec
}

func CPUSecsPerCycle() float64 {
	// Wait until estimateFrequency is done.
	for secsPerCycle == 0 {
	}
	return secsPerCycle
}

func CPUSecPerCycle() float64 {
	// Wait until estimateFrequency is done.
	for cyclesPerSec == 0 {
	}
	return cyclesPerSec
}

func measureCPUCyclesPerSec(wait float64) (freq float64) {
	var t0 [2]uint64
	var t1 [2]int64
	t1[0] = time.Now().UnixNano()
	t0[0] = Timestamp()
	time.Sleep(time.Duration(1e9 * wait))
	t1[1] = time.Now().UnixNano()
	t0[1] = Timestamp()
	freq = 1e9 * float64(t0[1]-t0[0]) / float64(t1[1]-t1[0])
	return
}

func round(x, unit float64) float64 {
	return unit * math.Floor(.5+x/unit)
}

func estimateFrequency(dt, unit, tolerance float64) {
	var sum, sum2, ave, rms, n float64
	for n = float64(1); true; n++ {
		f := measureCPUCyclesPerSec(dt)
		sum += f
		sum2 += f * f
		ave = sum / n
		rms = math.Sqrt((sum2/n - ave*ave) / n)
		if n >= 16 && rms < tolerance {
			break
		}
	}

	cyclesPerSec = round(ave, unit)
	secsPerCycle = 1 / cyclesPerSec
	return
}
