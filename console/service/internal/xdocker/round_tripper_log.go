package xdocker

import (
	"net/http"
	"postgresql-cluster-console/pkg/tracer"

	"github.com/docker/docker/client"
	"github.com/docker/go-connections/sockets"
	"github.com/rs/zerolog"
)

type roundTripperLog struct {
	http.Transport
	log zerolog.Logger
}

func NewRoundTripperLog(host string, log zerolog.Logger) (http.RoundTripper, error) {
	rt := &roundTripperLog{
		log: log,
	}

	hostURL, err := client.ParseHostURL(host)
	if err != nil {
		return nil, err
	}

	err = sockets.ConfigureTransport(&rt.Transport, hostURL.Scheme, hostURL.Host)
	if err != nil {
		return nil, err
	}

	return rt, nil
}

func (rt *roundTripperLog) RoundTrip(request *http.Request) (*http.Response, error) {
	localLog := rt.log.With().Str("cid", request.Context().Value(tracer.CtxCidKey{}).(string)).Logger()
	localLog.Trace().Str("url", request.URL.Path).Str("host", request.URL.Host).Str("method", request.Method).Msg("request")

	res, err := rt.Transport.RoundTrip(request)
	if err != nil {
		localLog.Error().Err(err).Msg("failed to RoundTrip")
	} else {
		localLog.Trace().Int("status", res.StatusCode).Msg("response")
	}

	return res, err
}
