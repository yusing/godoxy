package certapi

import (
	"net/http"

	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/logging/memlogger"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
)

func RenewCert(w http.ResponseWriter, r *http.Request) {
	autocert := config.GetInstance().AutoCertProvider()
	if autocert == nil {
		http.Error(w, "autocert is not enabled", http.StatusNotFound)
		return
	}

	conn, err := gpwebsocket.Initiate(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	//nolint:errcheck
	defer conn.CloseNow()

	logs, cancel := memlogger.Events()
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)
		err = autocert.ObtainCert()
		if err != nil {
			gperr.LogError("failed to obtain cert", err)
			gpwebsocket.WriteText(r, conn, err.Error())
		} else {
			logging.Info().Msg("cert obtained successfully")
		}
	}()
	for {
		select {
		case l := <-logs:
			if err != nil {
				return
			}
			if !gpwebsocket.WriteText(r, conn, string(l)) {
				return
			}
		case <-done:
			return
		}
	}
}
