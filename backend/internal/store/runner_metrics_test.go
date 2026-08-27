package store

import "testing"

func TestRunnerMetricsSampleValidation(t *testing.T) {
	valid := RunnerMetricsSample{CPUPercent: 0, MemoryPercent: 100, MemoryUsedBytes: 0, MemoryTotalBytes: 1}
	if err := valid.validate(); err != nil {
		t.Fatal(err)
	}
	for name, sample := range map[string]RunnerMetricsSample{
		"cpu":    {CPUPercent: 101, MemoryTotalBytes: 1},
		"memory": {MemoryPercent: -1, MemoryTotalBytes: 1},
		"size":   {MemoryTotalBytes: 0},
	} {
		t.Run(name, func(t *testing.T) {
			if err := sample.validate(); err == nil {
				t.Fatal("invalid runner metrics sample accepted")
			}
		})
	}
}
