// Copyright 2024 Andrew Sokolov
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package httpserver implements the vipcast REST API endpoints and handlers.
package httpserver

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/VictoriaMetrics/metrics"
	"github.com/asokolov365/vipcast/lib/appmetrics"
	"github.com/asokolov365/vipcast/lib/fasttime"
	"github.com/asokolov365/vipcast/lib/logging"
	"github.com/klauspost/compress/gzhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/valyala/fastrand"
)

var logger *zerolog.Logger

func Init() {
	if logger == nil {
		logger = logging.GetSubLogger("httpserver")
	}
}

var (
	metricsRequests          = metrics.NewCounter(`vipcast_http_requests_total{path="/metrics"}`)
	faviconRequests          = metrics.NewCounter(`vipcast_http_requests_total{path="*/favicon.ico"}`)
	requestsTotal            = metrics.NewCounter(`vipcast_http_requests_all_total`)
	unsupportedRequestErrors = metrics.NewCounter(`vipcast_http_request_errors_total{path="*", reason="unsupported"}`)

	metricsHandlerDuration = metrics.NewHistogram(`vipcast_http_request_duration_seconds{path="/metrics"}`)
	connTimeoutClosedConns = metrics.NewCounter(`vipcast_http_conn_timeout_closed_conns_total`)
)

var (
	disableResponseCompression = false
	idleConnTimeout            = 1 * time.Minute
	connTimeout                = 2 * time.Minute
)

// RequestHandler must serve the given request r and write response to w.
//
// RequestHandler must return true if the request has been served (successfully or not).
//
// RequestHandler must return false if it's not able to serve the given request.
// In such cases the caller must serve the request.
type RequestHandler func(w http.ResponseWriter, r *http.Request) bool

type Server struct {
	listenAddr string
}

func NewServer(addr string) *Server {
	var listenAddr string
	if addr == "" {
		listenAddr = "127.0.0.1:8179"
	} else if strings.HasPrefix(addr, ":") {
		listenAddr = "127.0.0.1" + addr
	} else {
		listenAddr = addr
	}
	return &Server{
		listenAddr: listenAddr,
	}
}

func (s *Server) Serve(ctx context.Context) {
	srv := &http.Server{
		Addr:              s.listenAddr,
		Handler:           gzipHandler(apiV1Handler),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       idleConnTimeout,
		// Do not set ReadTimeout and WriteTimeout here,
		// since these timeouts must be controlled by request handlers.

		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			timeoutSec := connTimeout.Seconds()
			// Add a jitter for connection timeout in order to prevent Thundering herd problem
			// when all the connections are established at the same time.
			// See https://en.wikipedia.org/wiki/Thundering_herd_problem
			jitterSec := fastrand.Uint32n(uint32(timeoutSec / 10))
			deadline := fasttime.UnixTimestamp() + uint64(timeoutSec) + uint64(jitterSec)
			return context.WithValue(ctx, connDeadlineTimeKey, &deadline)
		},
	}
	idleConnsClosed := make(chan struct{})
	go func() {
		<-ctx.Done()
		// Gracefully shutdown http server with 10s deadline
		deadLine := time.Now().Add(10 * time.Second)
		deadLineCtx, deadLineCtxCancel := context.WithDeadline(context.Background(), deadLine)
		logger.Info().Msgf("stopping API server at http://%s/", s.listenAddr)
		if err := srv.Shutdown(deadLineCtx); err != nil {
			// Error from closing listeners, or context timeout:
			logger.Error().Err(err).Msg("httpserver.Shutdown() failed")
		}
		deadLineCtxCancel()
		close(idleConnsClosed)
	}()
	logger.Info().Msgf("starting API server at http://%s/", s.listenAddr)
	logger.Info().Msgf("pprof handlers are exposed at http://%s/debug/pprof/", s.listenAddr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		// Error starting or closing listener
		logger.Error().Err(err).Msg("httpserver.ListenAndServe() failed")
	}
	<-idleConnsClosed
}

var connDeadlineTimeKey = interface{}("connDeadlineSecs")

func whetherToCloseConn(r *http.Request) bool {
	ctx := r.Context()
	v := ctx.Value(connDeadlineTimeKey)
	deadline, ok := v.(*uint64)
	return ok && fasttime.UnixTimestamp() > *deadline
}

func gzipHandler(rh RequestHandler) http.HandlerFunc {
	hf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerWrapper(w, r, rh)
	})
	if disableResponseCompression {
		return hf
	}
	return gzipHandlerWrapper(hf)
}

var gzipHandlerWrapper = func() func(http.Handler) http.HandlerFunc {
	hw, err := gzhttp.NewWrapper(gzhttp.CompressionLevel(1))
	if err != nil {
		panic(fmt.Errorf("BUG: unable to initialize gzip http wrapper: %w", err))
	}
	return hw
}()

var hostname = func() string {
	name, err := os.Hostname()
	if err != nil {
		log.Warn().Err(err).Msg("os.Hostname() failed")
		return "unknown"
	}
	return name
}()

func handlerWrapper(w http.ResponseWriter, r *http.Request, reqHandler RequestHandler) {
	// The standard net/http.Server recovers from panics in request handlers,
	// so vipcast state can become inconsistent after the recovered panic.
	// The following recover() code works around this by explicitly stopping the process after logging the panic.
	// See https://github.com/golang/go/issues/16542#issuecomment-246549902 for details.
	defer func() {
		if err := recover(); err != nil {
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, false)
			fmt.Fprintf(os.Stderr, "panic: %v\n\n%s", err, buf[:n])
			os.Exit(1)
		}
	}()

	h := w.Header()
	h.Add("X-Server-Hostname", hostname)
	requestsTotal.Inc()
	if whetherToCloseConn(r) {
		connTimeoutClosedConns.Inc()
		h.Set("Connection", "close")
	}
	if strings.HasSuffix(r.URL.Path, "/favicon.ico") {
		h.Set("Cache-Control", "max-age=3600")
		faviconRequests.Inc()
		w.Write(faviconData)
		return
	}
	switch r.URL.Path {
	case "/health":
		h.Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("OK"))
		return
	case "/metrics":
		metricsRequests.Inc()
		startTime := time.Now()
		h.Set("Content-Type", "text/plain; charset=utf-8")
		appmetrics.WritePrometheusMetrics(w)
		metricsHandlerDuration.UpdateDuration(startTime)
		return
	case "/robots.txt":
		// This prevents search engines from indexing contents
		fmt.Fprintf(w, "User-agent: *\nDisallow: /\n")
		return
	default:
		if strings.HasPrefix(r.URL.Path, "/debug/pprof/") {
			pprofHandler(r.URL.Path[len("/debug/pprof/"):], w, r)
			return
		}
		if reqHandler(w, r) {
			return
		}

		Errorf(w, r, "unsupported path requested: %q", r.URL.Path)
		unsupportedRequestErrors.Inc()
		return
	}
}

//go:embed favicon.ico
var faviconData []byte

// GetQuotedRemoteAddr returns quoted remote address.
func GetQuotedRemoteAddr(r *http.Request) string {
	remoteAddr := r.RemoteAddr
	if addr := r.Header.Get("X-Forwarded-For"); addr != "" {
		remoteAddr += ", X-Forwarded-For: " + addr
	}
	// quote remoteAddr and X-Forwarded-For, since they may contain untrusted input
	return strconv.Quote(remoteAddr)
}

// Errorf writes formatted error message to http.ResponseWriter and to the logger.
func Errorf(w http.ResponseWriter, r *http.Request, format string, args ...interface{}) {
	errStr := fmt.Sprintf(format, args...)
	remoteAddr := GetQuotedRemoteAddr(r)
	requestURI := GetRequestURI(r)
	logger.Warn().
		Str("remoteAddr", remoteAddr).
		Str("requestURI", requestURI).
		Msg(errStr)
	errStr = fmt.Sprintf("%s remoteAddr=%s requestURI=%q", errStr, remoteAddr, requestURI)

	// Extract statusCode from args
	statusCode := http.StatusBadRequest
	var esc *ErrorWithStatusCode
	for _, arg := range args {
		if err, ok := arg.(error); ok && errors.As(err, &esc) {
			statusCode = esc.StatusCode
			break
		}
	}
	http.Error(w, errStr, statusCode)
}

// ErrorWithStatusCode is an error with HTTP status code.
//
// The given StatusCode is sent to client when the error is passed to Errorf.
type ErrorWithStatusCode struct {
	Err        error
	StatusCode int
}

// Unwrap returns e.Err
//
// This is used by standard errors package. See https://golang.org/pkg/errors
func (e *ErrorWithStatusCode) Unwrap() error {
	return e.Err
}

// Error implements error interface.
func (e *ErrorWithStatusCode) Error() string {
	return e.Err.Error()
}

// WriteAPIHelp writes pathList to w in HTML format.
func WriteAPIHelp(w io.Writer, pathList [][2]string) {
	for _, p := range pathList {
		p, doc := p[0], p[1]
		fmt.Fprintf(w, "<a href=%q>%s</a> - %s<br/>", p, p, doc)
	}
}

// GetRequestURI returns requestURI
func GetRequestURI(r *http.Request) string {
	requestURI := r.RequestURI
	if r.Method != http.MethodPost {
		return requestURI
	}
	_ = r.ParseForm()
	queryArgs := r.PostForm.Encode()
	if len(queryArgs) == 0 {
		return requestURI
	}
	delimiter := "?"
	if strings.Contains(requestURI, delimiter) {
		delimiter = "&"
	}
	return requestURI + delimiter + queryArgs
}

// LogRequestError logs the errStr with the context from http.Request
func LogRequestError(r *http.Request, errStr string) {
	remoteAddr := GetQuotedRemoteAddr(r)
	requestURI := GetRequestURI(r)
	logger.Error().
		Str("remoteAddr", remoteAddr).
		Str("requestURI", requestURI).
		Msg(errStr)
}
