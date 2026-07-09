package autocert_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/goutils/task"
)

type countingTaskParent struct {
	*task.Task
	subtasks int
}

func (p *countingTaskParent) Subtask(name string, needFinish bool) *task.Task {
	p.subtasks++
	return p.Task.Subtask(name, needFinish)
}

func TestScheduleRenewalAllSkipsLocalAndPseudoProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider string
	}{
		{
			name:     "local",
			provider: autocert.ProviderLocal,
		},
		{
			name:     "pseudo",
			provider: autocert.ProviderPseudo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			provider, err := autocert.NewProvider(&autocert.Config{
				Provider: tt.provider,
				CertPath: filepath.Join(dir, "cert.pem"),
				KeyPath:  filepath.Join(dir, "key.pem"),
			}, nil, nil)
			require.NoError(t, err)

			parent := &countingTaskParent{Task: task.GetTestTask(t)}
			provider.ScheduleRenewalAll(parent)

			require.Zero(t, parent.subtasks)
		})
	}
}

func TestScheduleRenewalAllSkipsLocalAndPseudoExtraProviders(t *testing.T) {
	dir := t.TempDir()
	provider, err := autocert.NewProvider(&autocert.Config{
		Provider: autocert.ProviderCustom,
		CertPath: filepath.Join(dir, "main.pem"),
		KeyPath:  filepath.Join(dir, "main-key.pem"),
		Extra: []autocert.ConfigExtra{
			{
				Provider: autocert.ProviderLocal,
				CertPath: filepath.Join(dir, "local.pem"),
				KeyPath:  filepath.Join(dir, "local-key.pem"),
			},
			{
				Provider: autocert.ProviderPseudo,
				CertPath: filepath.Join(dir, "pseudo.pem"),
				KeyPath:  filepath.Join(dir, "pseudo-key.pem"),
			},
		},
	}, nil, nil)
	require.NoError(t, err)

	parent := &countingTaskParent{Task: task.GetTestTask(t)}
	provider.ScheduleRenewalAll(parent)

	require.Equal(t, 1, parent.subtasks)
}
