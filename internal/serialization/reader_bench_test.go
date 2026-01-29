package serialization

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// setupEnv sets up environment variables for benchmarks
func setupEnv(b *testing.B) {
	b.Helper()
	os.Setenv("BENCH_VAR", "benchmark_value")
	os.Setenv("BENCH_VAR_2", "second_value")
	os.Setenv("BENCH_VAR_3", "third_value")
}

// cleanupEnv cleans up environment variables after benchmarks
func cleanupEnv(b *testing.B) {
	b.Helper()
	os.Unsetenv("BENCH_VAR")
	os.Unsetenv("BENCH_VAR_2")
	os.Unsetenv("BENCH_VAR_3")
}

// BenchmarkSubstituteEnvReader_NoSubstitution benchmarks reading without any env substitutions
func BenchmarkSubstituteEnvReader_NoSubstitution(b *testing.B) {
	r := strings.NewReader(`key: value
name: test
data: some content here
`)

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_SingleSubstitution benchmarks reading with a single env substitution
func BenchmarkSubstituteEnvReader_SingleSubstitution(b *testing.B) {
	setupEnv(b)
	defer cleanupEnv(b)

	r := strings.NewReader(`key: ${BENCH_VAR}
`)

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_MultipleSubstitutions benchmarks reading with multiple env substitutions
func BenchmarkSubstituteEnvReader_MultipleSubstitutions(b *testing.B) {
	setupEnv(b)
	defer cleanupEnv(b)

	r := strings.NewReader(`key1: ${BENCH_VAR}
key2: ${BENCH_VAR_2}
key3: ${BENCH_VAR_3}
`)

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_LargeInput_NoSubstitution benchmarks large input without substitutions
func BenchmarkSubstituteEnvReader_LargeInput_NoSubstitution(b *testing.B) {
	r := strings.NewReader(strings.Repeat("x", 100000))

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_LargeInput_WithSubstitutions benchmarks large input with scattered substitutions
func BenchmarkSubstituteEnvReader_LargeInput_WithSubstitutions(b *testing.B) {
	setupEnv(b)
	defer cleanupEnv(b)

	var builder bytes.Buffer
	for range 100 {
		builder.WriteString(strings.Repeat("x", 1000))
		builder.WriteString("${BENCH_VAR}")
	}
	r := bytes.NewReader(builder.Bytes())

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_SmallBuffer benchmarks reading with a small buffer size
func BenchmarkSubstituteEnvReader_SmallBuffer(b *testing.B) {
	setupEnv(b)
	defer cleanupEnv(b)

	r := strings.NewReader(`key: ${BENCH_VAR} and some more content here`)
	buf := make([]byte, 16)

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		for {
			_, err := reader.Read(buf)
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_YAMLConfig benchmarks a realistic YAML config scenario
func BenchmarkSubstituteEnvReader_YAMLConfig(b *testing.B) {
	setupEnv(b)
	defer cleanupEnv(b)

	r := strings.NewReader(`database:
  host: ${BENCH_VAR}
  port: ${BENCH_VAR_2}
  username: ${BENCH_VAR_3}
  password: ${BENCH_VAR}
cache:
  enabled: true
  ttl: ${BENCH_VAR_2}
server:
  host: ${BENCH_VAR}
  port: 8080
`)

	b.ResetTimer()
	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_BoundaryPattern benchmarks patterns at buffer boundaries (4096 bytes)
func BenchmarkSubstituteEnvReader_BoundaryPattern(b *testing.B) {
	setupEnv(b)
	defer cleanupEnv(b)

	// Pattern exactly at 4090 bytes, with ${VAR} crossing the 4096 boundary
	prefix := strings.Repeat("x", 4090)
	r := strings.NewReader(prefix + "${BENCH_VAR}")

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_MultipleBoundaries benchmarks multiple patterns crossing boundaries
func BenchmarkSubstituteEnvReader_MultipleBoundaries(b *testing.B) {
	setupEnv(b)
	defer cleanupEnv(b)

	var builder bytes.Buffer
	for range 10 {
		builder.WriteString(strings.Repeat("x", 4000))
		builder.WriteString("${BENCH_VAR}")
	}
	r := bytes.NewReader(builder.Bytes())

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_SpecialChars benchmarks substitution with special characters
func BenchmarkSubstituteEnvReader_SpecialChars(b *testing.B) {
	os.Setenv("SPECIAL_BENCH_VAR", `value with "quotes" and \backslash\`)
	defer os.Unsetenv("SPECIAL_BENCH_VAR")

	r := strings.NewReader(`key: ${SPECIAL_BENCH_VAR}
`)

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_EmptyValue benchmarks substitution with empty value
func BenchmarkSubstituteEnvReader_EmptyValue(b *testing.B) {
	os.Setenv("EMPTY_BENCH_VAR", "")
	defer os.Unsetenv("EMPTY_BENCH_VAR")

	r := strings.NewReader(`key: ${EMPTY_BENCH_VAR}
`)

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkSubstituteEnvReader_DollarWithoutBrace benchmarks $ without following {
func BenchmarkSubstituteEnvReader_DollarWithoutBrace(b *testing.B) {
	os.Setenv("BENCH_VAR", "benchmark_value")
	defer os.Unsetenv("BENCH_VAR")

	r := strings.NewReader(`price: $100 and $200 for ${BENCH_VAR}`)

	for b.Loop() {
		reader := NewSubstituteEnvReader(r)
		_, err := io.ReadAll(reader)
		if err != nil {
			b.Fatal(err)
		}
		r.Seek(0, io.SeekStart)
	}
}

// BenchmarkFindIncompletePatternStart benchmarks the findIncompletePatternStart function
func BenchmarkFindIncompletePatternStart(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"no pattern", strings.Repeat("hello world ", 100)},
		{"complete pattern", strings.Repeat("hello ${VAR} world ", 50)},
		{"dollar at end", strings.Repeat("hello ", 100) + "$"},
		{"incomplete at end", strings.Repeat("hello ", 100) + "${VAR"},
		{"large input no pattern", strings.Repeat("x", 5000)},
		{"large input with pattern", strings.Repeat("x", 4000) + "${VAR}"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			data := []byte(tc.input)
			for b.Loop() {
				findIncompletePatternStart(data)
			}
		})
	}
}
