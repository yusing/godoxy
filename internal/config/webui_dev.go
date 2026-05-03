//go:build !production

package config

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/logging"
	"github.com/yusing/goutils/task"
)

const (
	webUIDevServerHost       = "127.0.0.1"
	webUIDevServerPort       = 5173
	defaultWebUIDevServerURL = "http://127.0.0.1:5173"
)

var webUIDevServerOnce sync.Once

func webUIDevServerURL() (string, int, bool, error) {
	if common.IsTest {
		return "", 0, false, nil
	}
	if _, err := os.Stat(filepath.Join("webui", "package.json")); err != nil {
		return "", 0, false, err
	}
	webUIDevServerOnce.Do(func() {
		go func() {
			if err := startWebUIDevServer(); err != nil {
				log.Warn().Err(err).Msg("webui error")
			}
		}()
	})
	return webUIDevServerHost, webUIDevServerPort, true, nil
}

func startWebUIDevServer() error {
	if ready(task.RootContext(), defaultWebUIDevServerURL) {
		log.Info().Str("target", defaultWebUIDevServerURL).Msg("using existing WebUI Vite dev server")
		return nil
	}

	cmd := exec.CommandContext(task.RootContext(), "bun", "run", "dev", "--", "--host", webUIDevServerHost, "--port", strconv.Itoa(webUIDevServerPort), "--strictPort")
	cmd.Dir = "webui"
	cmd.Env = append(os.Environ(), "BROWSER=none")

	cmd.Stdout = logging.NewLoggerWithFixedLevel(zerolog.DebugLevel)
	cmd.Stderr = logging.NewLoggerWithFixedLevel(zerolog.InfoLevel) // somehow vite will output to stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start WebUI Vite dev server: %w", err)
	}
	task.OnProgramExit("stop WebUI Vite dev server", func() {
		_ = cmd.Process.Kill()
	})

	probeCtx, probeCancel := context.WithCancel(task.RootContext())
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		probeCancel()
	}()

	for {
		select {
		case err := <-exited:
			return fmt.Errorf("WebUI Vite dev server exited before ready: %w", err)
		case <-probeCtx.Done():
			return fmt.Errorf("WebUI Vite dev server exited before ready: %w", probeCtx.Err())
		case <-time.After(time.Second):
			if ready(probeCtx, defaultWebUIDevServerURL) {
				log.Info().Str("target", defaultWebUIDevServerURL).Msg("started WebUI Vite dev server")
				return nil
			}
		}
	}
}

func ready(ctx context.Context, target string) bool {
	client := http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}
