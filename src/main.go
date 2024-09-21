package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/yusing/go-proxy/api"
	apiUtils "github.com/yusing/go-proxy/api/v1/utils"
	"github.com/yusing/go-proxy/common"
	"github.com/yusing/go-proxy/config"
	"github.com/yusing/go-proxy/docker"
	"github.com/yusing/go-proxy/docker/idlewatcher"
	E "github.com/yusing/go-proxy/error"
	R "github.com/yusing/go-proxy/route"
	"github.com/yusing/go-proxy/server"
	F "github.com/yusing/go-proxy/utils/functional"
	W "github.com/yusing/go-proxy/watcher"
)

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	args := common.GetArgs()
	l := logrus.WithField("module", "main")

	if common.IsDebug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		DisableSorting:         true,
		DisableLevelTruncation: true,
		FullTimestamp:          true,
		ForceColors:            true,
		TimestampFormat:        "01-02 15:04:05",
	})

	if args.Command == common.CommandReload {
		if err := apiUtils.ReloadServer(); err.HasError() {
			l.Fatal(err)
		}
		return
	}

	onShutdown := F.NewSlice[func()]()

	// exit if only validate config
	if args.Command == common.CommandValidate {
		data, err := os.ReadFile(common.ConfigPath)
		if err == nil {
			err = config.Validate(data).Error()
		}
		if err != nil {
			l.Fatal("config error: ", err)
		}
		l.Printf("config OK")
		return
	}

	cfg, err := config.Load()
	if err.IsFatal() {
		l.Fatal(err)
	}

	if args.Command == common.CommandListConfigs {
		printJSON(cfg.Value())
		return
	}

	cfg.StartProxyProviders()

	if args.Command == common.CommandListRoutes {
		printJSON(cfg.RoutesByAlias())
		return
	}

	if args.Command == common.CommandDebugListEntries {
		printJSON(cfg.DumpEntries())
		return
	}

	if err.HasError() {
		l.Warn(err)
	}

	W.InitFileWatcherHelper()
	cfg.WatchChanges()

	onShutdown.Add(docker.CloseAllClients)
	onShutdown.Add(cfg.Dispose)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT)
	signal.Notify(sig, syscall.SIGTERM)
	signal.Notify(sig, syscall.SIGHUP)

	autocert := cfg.GetAutoCertProvider()

	if autocert != nil {
		if err = autocert.LoadCert(); err.HasError() {
			if !err.Is(os.ErrNotExist) { // ignore if cert doesn't exist
				l.Error(err)
			}
			l.Debug("obtaining cert due to error loading cert")
			if err = autocert.ObtainCert(); err.HasError() {
				l.Warn(err)
			}
		}

		if err.NoError() {
			ctx, certRenewalCancel := context.WithCancel(context.Background())
			go autocert.ScheduleRenewal(ctx)
			onShutdown.Add(certRenewalCancel)
		}

		for _, expiry := range autocert.GetExpiries() {
			l.Infof("certificate expire on %s", expiry)
			break
		}
	} else {
		l.Info("autocert not configured")
	}

	proxyServer := server.InitProxyServer(server.Options{
		Name:            "proxy",
		CertProvider:    autocert,
		HTTPPort:        common.ProxyHTTPPort,
		HTTPSPort:       common.ProxyHTTPSPort,
		Handler:         http.HandlerFunc(R.ProxyHandler),
		RedirectToHTTPS: cfg.Value().RedirectToHTTPS,
	})
	apiServer := server.InitAPIServer(server.Options{
		Name:            "api",
		CertProvider:    autocert,
		HTTPPort:        common.APIHTTPPort,
		Handler:         api.NewHandler(cfg),
		RedirectToHTTPS: cfg.Value().RedirectToHTTPS,
	})

	proxyServer.Start()
	apiServer.Start()
	onShutdown.Add(proxyServer.Stop)
	onShutdown.Add(apiServer.Stop)

	go idlewatcher.Start()
	onShutdown.Add(idlewatcher.Stop)

	// wait for signal
	<-sig

	// grafully shutdown
	logrus.Info("shutting down")
	done := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Add(onShutdown.Size())
	onShutdown.ForEach(func(f func()) {
		go func() {
			f()
			wg.Done()
		}()
	})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logrus.Info("shutdown complete")
	case <-time.After(time.Duration(cfg.Value().TimeoutShutdown) * time.Second):
		logrus.Info("timeout waiting for shutdown")
	}
}

func printJSON(obj any) {
	j, err := E.Check(json.Marshal(obj))
	if err.HasError() {
		logrus.Fatal(err)
	}
	rawLogger := log.New(os.Stdout, "", 0)
	rawLogger.Printf("%s", j) // raw output for convenience using "jq"
}
