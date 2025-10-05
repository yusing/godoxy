package systeminfo

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"
	"github.com/yusing/goutils/num"
	expect "github.com/yusing/goutils/testing"
)

// Create test data
var (
	cpuAvg   = 45.67
	testInfo = &SystemInfo{
		Timestamp:  123456,
		CPUAverage: &cpuAvg,
		Memory: mem.VirtualMemoryStat{
			Total:       16000000000,
			Available:   8000000000,
			Used:        8000000000,
			UsedPercent: 50.0,
		},
		Disks: map[string]disk.UsageStat{
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
		DisksIO: map[string]*disk.IOCountersStat{
			"media": {
				Name:       "media",
				ReadBytes:  1000000,
				WriteBytes: 2000000,
				IOCountersStatExtra: disk.IOCountersStatExtra{
					ReadSpeed:  100.5,
					WriteSpeed: 200.5,
					Iops:       1000,
				},
			},
			"nvme0n1": {
				Name:       "nvme0n1",
				ReadBytes:  1000000,
				WriteBytes: 2000000,
				IOCountersStatExtra: disk.IOCountersStatExtra{
					ReadSpeed:  100.5,
					WriteSpeed: 200.5,
					Iops:       1000,
				},
			},
		},
		Network: net.IOCountersStat{
			BytesSent:     5000000,
			BytesRecv:     10000000,
			UploadSpeed:   1024.5,
			DownloadSpeed: 2048.5,
		},
		Sensors: []sensors.TemperatureStat{
			{
				SensorKey:   "cpu_temp",
				Temperature: num.NewPercentage(30.0),
				High:        num.NewPercentage(40.0),
				Critical:    num.NewPercentage(50.0),
			},
			{
				SensorKey:   "gpu_temp",
				Temperature: num.NewPercentage(40.0),
				High:        num.NewPercentage(50.0),
				Critical:    num.NewPercentage(60.0),
			},
		},
	}
)

func TestSystemInfo(t *testing.T) {
	// Test marshaling
	data, err := sonic.Marshal(testInfo)
	expect.NoError(t, err)

	// Test unmarshaling back
	var decoded SystemInfo
	err = sonic.Unmarshal(data, &decoded)
	expect.NoError(t, err)

	// Compare original and decoded
	expect.Equal(t, decoded.Timestamp, testInfo.Timestamp)
	expect.Equal(t, *decoded.CPUAverage, *testInfo.CPUAverage)
	expect.Equal(t, decoded.Memory, testInfo.Memory)
	expect.Equal(t, decoded.Disks, testInfo.Disks)
	expect.Equal(t, decoded.DisksIO, testInfo.DisksIO)
	expect.Equal(t, decoded.Network, testInfo.Network)
	expect.Equal(t, decoded.Sensors, testInfo.Sensors)

	// Test nil fields
	nilInfo := &SystemInfo{
		Timestamp: 1234567890,
	}

	data, err = json.Marshal(nilInfo)
	expect.NoError(t, err)

	var decodedNil SystemInfo
	err = json.Unmarshal(data, &decodedNil)
	expect.NoError(t, err)

	expect.Equal(t, decodedNil.Timestamp, nilInfo.Timestamp)
	// expect.True(t, decodedNil.CPUAverage == nil)
	// expect.True(t, decodedNil.Memory == nil)
	// expect.True(t, decodedNil.Disks == nil)
	// expect.True(t, decodedNil.Network == nil)
	// expect.True(t, decodedNil.Sensors == nil)
}

func TestSerialize(t *testing.T) {
	entries := make([]*SystemInfo, 5)
	for i := range 5 {
		entries[i] = testInfo
	}
	for _, query := range allQueries {
		t.Run(string(query), func(t *testing.T) {
			_, result := aggregate(entries, url.Values{"aggregate": []string{string(query)}})
			s, err := sonic.Marshal(result)
			expect.NoError(t, err)
			var v []map[string]any
			expect.NoError(t, json.Unmarshal(s, &v))
			expect.Equal(t, len(v), len(result.Entries))
			for i, m := range v {
				for k, v := range m {
					vv := reflect.ValueOf(result.Entries[i][k])
					expect.Equal(t, reflect.ValueOf(v).Interface(), vv.Interface())
				}
			}
		})
	}
}
