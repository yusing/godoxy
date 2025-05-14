package systeminfo

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/shirou/gopsutil/v4/sensors"
	"github.com/yusing/go-proxy/internal/utils/synk"
)

var bufPool = synk.NewBytesPool(1024, 16384)

// explicitly implement MarshalJSON to avoid reflection.
func (s *SystemInfo) MarshalJSON() ([]byte, error) {
	b := bufPool.Get()
	defer bufPool.Put(b)

	b = append(b, '{')

	// timestamp
	b = append(b, `"timestamp":`...)
	b = strconv.AppendInt(b, s.Timestamp, 10)

	// cpu_average
	b = append(b, `,"cpu_average":`...)
	if s.CPUAverage != nil {
		b = strconv.AppendFloat(b, *s.CPUAverage, 'f', 2, 64)
	} else {
		b = append(b, "null"...)
	}

	// memory
	b = append(b, `,"memory":`...)
	if s.Memory != nil {
		b = fmt.Appendf(b,
			`{"total":%d,"available":%d,"used":%d,"used_percent":%.2f}`,
			s.Memory.Total,
			s.Memory.Available,
			s.Memory.Used,
			s.Memory.UsedPercent,
		)
	} else {
		b = append(b, "null"...)
	}

	// disk
	b = append(b, `,"disks":`...)
	if len(s.Disks) > 0 {
		b = append(b, '{')
		first := true
		for device, disk := range s.Disks {
			if !first {
				b = append(b, ',')
			}
			b = fmt.Appendf(b,
				`"%s":{"device":"%s","path":"%s","fstype":"%s","total":%d,"free":%d,"used":%d,"used_percent":%.2f}`,
				device,
				device,
				disk.Path,
				disk.Fstype,
				disk.Total,
				disk.Free,
				disk.Used,
				disk.UsedPercent,
			)
			first = false
		}
		b = append(b, '}')
	} else {
		b = append(b, "null"...)
	}

	// disks_io
	b = append(b, `,"disks_io":`...)
	if len(s.DisksIO) > 0 {
		b = append(b, '{')
		first := true
		for name, usage := range s.DisksIO {
			if !first {
				b = append(b, ',')
			}
			b = fmt.Appendf(b,
				`"%s":{"name":"%s","read_bytes":%d,"write_bytes":%d,"read_speed":%.2f,"write_speed":%.2f,"iops":%d}`,
				name,
				name,
				usage.ReadBytes,
				usage.WriteBytes,
				usage.ReadSpeed,
				usage.WriteSpeed,
				usage.Iops,
			)
			first = false
		}
		b = append(b, '}')
	} else {
		b = append(b, "null"...)
	}

	// network
	b = append(b, `,"network":`...)
	if s.Network != nil {
		b = fmt.Appendf(b,
			`{"bytes_sent":%d,"bytes_recv":%d,"upload_speed":%.2f,"download_speed":%.2f}`,
			s.Network.BytesSent,
			s.Network.BytesRecv,
			s.Network.UploadSpeed,
			s.Network.DownloadSpeed,
		)
	} else {
		b = append(b, "null"...)
	}

	// sensors
	b = append(b, `,"sensors":`...)
	if len(s.Sensors) > 0 {
		b = append(b, '{')
		first := true
		for _, sensor := range s.Sensors {
			if !first {
				b = append(b, ',')
			}
			b = fmt.Appendf(b,
				`"%s":{"name":"%s","temperature":%.2f,"high":%.2f,"critical":%.2f}`,
				sensor.SensorKey,
				sensor.SensorKey,
				sensor.Temperature,
				sensor.High,
				sensor.Critical,
			)
			first = false
		}
		b = append(b, '}')
	} else {
		b = append(b, "null"...)
	}

	b = append(b, '}')
	return b, nil
}

func (s *Sensors) UnmarshalJSON(data []byte) error {
	var v map[string]map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if len(v) == 0 {
		return nil
	}
	*s = make(Sensors, 0, len(v))
	for k, v := range v {
		*s = append(*s, sensors.TemperatureStat{
			SensorKey:   k,
			Temperature: v["temperature"].(float64),
			High:        v["high"].(float64),
			Critical:    v["critical"].(float64),
		})
	}
	return nil
}

func (result Aggregated) MarshalJSON() ([]byte, error) {
	buf := bufPool.Get()
	defer bufPool.Put(buf)

	buf = append(buf, '[')
	i := 0
	n := len(result)
	for _, entry := range result {
		buf = append(buf, '{')
		j := 0
		m := len(entry)
		for k, v := range entry {
			buf = append(buf, '"')
			buf = append(buf, k...)
			buf = append(buf, '"')
			buf = append(buf, ':')
			switch v := v.(type) {
			case float64:
				buf = strconv.AppendFloat(buf, v, 'f', 2, 64)
			case uint64:
				buf = strconv.AppendUint(buf, v, 10)
			case int64:
				buf = strconv.AppendInt(buf, v, 10)
			default:
				panic(fmt.Sprintf("unexpected type: %T", v))
			}
			if j != m-1 {
				buf = append(buf, ',')
			}
			j++
		}
		buf = append(buf, '}')
		if i != n-1 {
			buf = append(buf, ',')
		}
		i++
	}
	buf = append(buf, ']')
	return buf, nil
}
