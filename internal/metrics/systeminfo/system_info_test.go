package systeminfo

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"

	"github.com/shirou/gopsutil/v4/sensors"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

// Create test data
var cpuAvg = 45.67
var testInfo = &SystemInfo{
	Timestamp:  123456,
	CPUAverage: &cpuAvg,
	Memory: &MemoryUsage{
		Total:       16000000000,
		Available:   8000000000,
		Used:        8000000000,
		UsedPercent: 50.0,
	},
	Disks: map[string]*Disk{
		"sda": {
			Path:        "/",
			Fstype:      "ext4",
			Total:       500000000000,
			Free:        250000000000,
			Used:        250000000000,
			UsedPercent: 50.0,
		},
		"nvme0n1": {
			Path:        "/",
			Fstype:      "zfs",
			Total:       500000000000,
			Free:        250000000000,
			Used:        250000000000,
			UsedPercent: 50.0,
		},
	},
	DisksIO: map[string]*DiskIO{
		"media": {
			ReadBytes:  1000000,
			WriteBytes: 2000000,
			ReadSpeed:  100.5,
			WriteSpeed: 200.5,
			Iops:       1000,
		},
		"nvme0n1": {
			ReadBytes:  1000000,
			WriteBytes: 2000000,
			ReadSpeed:  100.5,
			WriteSpeed: 200.5,
			Iops:       1000,
		},
	},
	Network: &Network{
		BytesSent:     5000000,
		BytesRecv:     10000000,
		UploadSpeed:   1024.5,
		DownloadSpeed: 2048.5,
	},
	Sensors: []sensors.TemperatureStat{
		{
			SensorKey:   "cpu_temp",
			Temperature: 30.0,
			High:        40.0,
			Critical:    50.0,
		},
		{
			SensorKey:   "gpu_temp",
			Temperature: 40.0,
			High:        50.0,
			Critical:    60.0,
		},
	},
}

func TestSystemInfo(t *testing.T) {
	// Test marshaling
	data, err := json.Marshal(testInfo)
	ExpectNoError(t, err)

	// Test unmarshaling back
	var decoded SystemInfo
	err = json.Unmarshal(data, &decoded)
	ExpectNoError(t, err)

	// Compare original and decoded
	ExpectEqual(t, decoded.Timestamp, testInfo.Timestamp)
	ExpectEqual(t, *decoded.CPUAverage, *testInfo.CPUAverage)
	ExpectDeepEqual(t, decoded.Memory, testInfo.Memory)
	ExpectDeepEqual(t, decoded.Disks, testInfo.Disks)
	ExpectDeepEqual(t, decoded.DisksIO, testInfo.DisksIO)
	ExpectDeepEqual(t, decoded.Network, testInfo.Network)
	ExpectDeepEqual(t, decoded.Sensors, testInfo.Sensors)

	// Test nil fields
	nilInfo := &SystemInfo{
		Timestamp: 1234567890,
	}

	data, err = json.Marshal(nilInfo)
	ExpectNoError(t, err)

	var decodedNil SystemInfo
	err = json.Unmarshal(data, &decodedNil)
	ExpectNoError(t, err)

	ExpectDeepEqual(t, decodedNil.Timestamp, nilInfo.Timestamp)
	ExpectTrue(t, decodedNil.CPUAverage == nil)
	ExpectTrue(t, decodedNil.Memory == nil)
	ExpectTrue(t, decodedNil.Disks == nil)
	ExpectTrue(t, decodedNil.Network == nil)
	ExpectTrue(t, decodedNil.Sensors == nil)
}

func TestSerialize(t *testing.T) {
	entries := make([]*SystemInfo, 5)
	for i := 0; i < 5; i++ {
		entries[i] = testInfo
	}
	for _, query := range allQueries {
		t.Run(query, func(t *testing.T) {
			_, result := aggregate(entries, url.Values{"aggregate": []string{query}})
			s, err := result.MarshalJSON()
			ExpectNoError(t, err)
			var v []map[string]any
			ExpectNoError(t, json.Unmarshal(s, &v))
			ExpectEqual(t, len(v), len(result))
			for i, m := range v {
				for k, v := range m {
					// some int64 values are converted to float64 on json.Unmarshal
					vv := reflect.ValueOf(result[i][k])
					ExpectEqual(t, reflect.ValueOf(v).Convert(vv.Type()).Interface(), vv.Interface())
				}
			}
		})
	}
}

func BenchmarkSerialize(b *testing.B) {
	entries := make([]*SystemInfo, b.N)
	for i := 0; i < b.N; i++ {
		entries[i] = testInfo
	}
	queries := map[string]Aggregated{}
	for _, query := range allQueries {
		_, result := aggregate(entries, url.Values{"aggregate": []string{query}})
		queries[query] = result
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.Run("optimized", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, query := range allQueries {
				_, _ = queries[query].MarshalJSON()
			}
		}
	})
	b.Run("json", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			for _, query := range allQueries {
				_, _ = json.Marshal([]map[string]any(queries[query]))
			}
		}
	})
}
