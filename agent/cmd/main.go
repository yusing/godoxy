package main

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/env"
	"github.com/yusing/godoxy/agent/pkg/server"
	"github.com/yusing/godoxy/internal/metrics/systeminfo"
	"github.com/yusing/godoxy/pkg"
	socketproxy "github.com/yusing/godoxy/socketproxy/pkg"
	httpServer "github.com/yusing/goutils/server"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/task"
)

func main() {
	writer := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "01-02 15:04",
	}
	zerolog.TimeFieldFormat = writer.TimeFormat
	log.Logger = zerolog.New(writer).Level(zerolog.InfoLevel).With().Timestamp().Logger()
	ca := &agent.PEMPair{}
	err := ca.Load(env.AgentCACert)
	if err != nil {
		log.Fatal().Err(err).Msg("init CA error")
	}
	caCert, err := ca.ToTLSCert()
	if err != nil {
		log.Fatal().Err(err).Msg("init CA error")
	}

	srv := &agent.PEMPair{}
	srv.Load(env.AgentSSLCert)
	if err != nil {
		log.Fatal().Err(err).Msg("init SSL error")
	}
	srvCert, err := srv.ToTLSCert()
	if err != nil {
		log.Fatal().Err(err).Msg("init SSL error")
	}

	log.Info().Msgf("GoDoxy Agent version %s", pkg.GetVersion())
	log.Info().Msgf("Agent name: %s", env.AgentName)
	log.Info().Msgf("Agent port: %d", env.AgentPort)
	log.Info().Msgf("Agent runtime: %s", env.Runtime)

	log.Info().Msg(`
Tips:
1. To change the agent name, you can set the AGENT_NAME environment variable.
2. To change the agent port, you can set the AGENT_PORT environment variable.
`)

	t := task.RootTask("agent", false)
	opts := server.Options{
		CACert:     caCert,
		ServerCert: srvCert,
		Port:       env.AgentPort,
	}

	server.StartAgentServer(t, opts)

	if socketproxy.ListenAddr != "" {
		runtime := strutils.Title(string(env.Runtime))

		log.Info().Msgf("%s socket listening on: %s", runtime, socketproxy.ListenAddr)
		opts := httpServer.Options{
			Name:     runtime,
			HTTPAddr: socketproxy.ListenAddr,
			Handler:  socketproxy.NewHandler(),
		}
		httpServer.StartServer(t, opts)
	}

	systeminfo.Poller.Start()

	task.WaitExit(3)
}
