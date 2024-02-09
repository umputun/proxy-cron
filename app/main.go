package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/go-pkgz/lgr"
	"github.com/go-pkgz/rest"
	"github.com/go-pkgz/rest/logger"
	"github.com/robfig/cron"
	"github.com/umputun/go-flags"
)

type options struct {
	Port int `long:"port" env:"PORT" default:"8080" description:"port to listen on"`

	Timeout struct {
		Connect time.Duration `long:"connect" env:"CONNECT" default:"10s" description:"connect timeout"`
		Read    time.Duration `long:"read" env:"READ" default:"10s" description:"read timeout"`
		Write   time.Duration `long:"write" env:"WRITE" default:"10s" description:"write timeout"`
		Idle    time.Duration `long:"idle" env:"IDLE" default:"15s" description:"idle timeout"`
	} `group:"timeout" namespace:"timeout" env-namespace:"TIMEOUT"`

	MaxBodySize     int64 `long:"max-size" env:"MAX_SIZE" default:"1048576" description:"max body size in bytes"`
	SuppressHeaders bool  `long:"suppress-headers" env:"SUPPRESS_HEADERS" description:"suppress custom proxy-cron headers in the response"`

	NoColors bool `long:"no-colors" env:"NO_COLORS" description:"disable colorized logging"`
	Dbg      bool `long:"dbg" env:"DEBUG" description:"debug mode"`
}

var revision = "unknown"

func main() {
	fmt.Printf("proxy-cron %s\n", revision)
	opts := options{}
	p := flags.NewParser(&opts, flags.PrintErrors|flags.PassDoubleDash|flags.HelpFlag)
	p.SubcommandsOptional = true
	if _, err := p.Parse(); err != nil {
		if err.(*flags.Error).Type != flags.ErrHelp {
			fmt.Printf("%v", err)
		}
		os.Exit(1)
	}
	setupLog(opts.Dbg, opts.NoColors)
	catchSignal()

	log.Printf("[DEBUG] options: %+v", opts)

	defer func() {
		if x := recover(); x != nil {
			log.Printf("[WARN] run time panic:\n%v", x)
			panic(x)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx, opts); err != nil {
		log.Printf("[FATAL] failed: %v", err)
	}
}

// run starts the proxy server
// it listens for incoming requests and proxies them to the specified endpoint based on a crontab schedule.
// if the request is within the allowed time, the response is fetched from the endpoint and cached.
// if the request is outside the allowed time, the cached response is returned.
// handles GET requests only
func run(ctx context.Context, opts options) error {
	log.Printf("[INFO] proxy is running on port %d", opts.Port)

	l := logger.New(logger.Log(lgr.Default()), logger.Prefix("[INFO]"))

	middlewares := []func(http.Handler) http.Handler{rest.AppInfo("proxy-cron", "umputun", revision),
		rest.RealIP, rest.Recoverer(lgr.Default()), rest.Ping, l.Handler}
	if opts.SuppressHeaders {
		// remove first middleware, which is AppInfo responsible for adding custom headers
		middlewares = middlewares[1:]
	}
	handler := rest.Wrap(http.HandlerFunc(proxyHandler(opts.MaxBodySize)), middlewares...)
	srv := http.Server{
		Addr:         fmt.Sprintf(":%d", opts.Port),
		Handler:      handler,
		ReadTimeout:  opts.Timeout.Read,
		WriteTimeout: opts.Timeout.Write,
		IdleTimeout:  opts.Timeout.Idle,
	}

	go func() {
		<-ctx.Done()
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("[WARN] server shutdown failed:%+v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server failed: %w", err)
	}
	return nil
}

// cachedResponse stores the last response for each endpoint
type cachedResponse struct {
	body    string
	headers http.Header
}

var (
	responseCache = make(map[string]cachedResponse)
	cacheMutex    = &sync.RWMutex{}
	client        = &http.Client{Timeout: 10 * time.Second}
)

// proxyHandler handles incoming proxy requests
func proxyHandler(maxBodySize int64) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		endpoint := r.URL.Query().Get("endpoint")
		crontab := r.URL.Query().Get("crontab")
		crontab = strings.ReplaceAll(crontab, "_", " ") // replace underscores with spaces to allow easier crontab input

		allowed, err := isAllowedTime(crontab)
		if err != nil {
			log.Printf("[WARN] failed to check if request is allowed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if allowed {
			// request is within the allowed time, fetch the response from the endpoint
			resp, err := client.Get(endpoint)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer resp.Body.Close()

			limitedReader := io.LimitReader(resp.Body, maxBodySize) // limit the response body to 1MB
			body, err := io.ReadAll(limitedReader)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			responseBody := string(body)

			// update the cache
			cacheMutex.Lock()
			responseCache[endpoint] = cachedResponse{body: responseBody, headers: resp.Header}
			cacheMutex.Unlock()

			// copy original headers to the response writer
			copyHeaders(w.Header(), resp.Header)
			if _, err := fmt.Fprint(w, responseBody); err != nil {
				log.Printf("[WARN] failed to write response: %v", err)
				return
			}
			log.Printf("[DEBUG] non-cached response from %s: %s", endpoint, strings.ReplaceAll(responseBody, "\n", " "))
			return
		}

		// request outside the allowed time, return the cached response
		cacheMutex.RLock()
		cachedResponse, ok := responseCache[endpoint]
		cacheMutex.RUnlock()

		if !ok {
			http.Error(w, "No cached response available", http.StatusNotFound)
			return
		}

		// copy cached headers to the response writer
		copyHeaders(w.Header(), cachedResponse.headers)
		if _, err := fmt.Fprint(w, cachedResponse.body); err != nil {
			log.Printf("[WARN] failed to write response: %v", err)
			return
		}
		log.Printf("[DEBUG] cached response from %s: %s", endpoint, strings.ReplaceAll(cachedResponse.body, "\n", " "))
	}
}

// isAllowedTime checks if the current time falls within the crontab schedule
func isAllowedTime(crontab string) (bool, error) {
	scheduler := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := scheduler.Parse(crontab)
	if err != nil {
		return false, fmt.Errorf("failed to parse crontab: %w", err)
	}

	now := time.Now()
	nextTime := schedule.Next(now)
	diff := nextTime.Sub(now)
	return now.Before(nextTime) && diff <= time.Minute, nil
}

// copyHeaders copies headers from source to destination
func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func setupLog(dbg, noColors bool) {
	logOpts := []lgr.Option{lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	if dbg {
		logOpts = []lgr.Option{lgr.Debug, lgr.CallerFile, lgr.CallerFunc, lgr.Msec, lgr.LevelBraces, lgr.StackTraceOnError}
	}

	if !noColors {
		colorizer := lgr.Mapper{
			ErrorFunc:  func(s string) string { return color.New(color.FgHiRed).Sprint(s) },
			WarnFunc:   func(s string) string { return color.New(color.FgRed).Sprint(s) },
			InfoFunc:   func(s string) string { return color.New(color.FgYellow).Sprint(s) },
			DebugFunc:  func(s string) string { return color.New(color.FgWhite).Sprint(s) },
			CallerFunc: func(s string) string { return color.New(color.FgBlue).Sprint(s) },
			TimeFunc:   func(s string) string { return color.New(color.FgCyan).Sprint(s) },
		}
		logOpts = append(logOpts, lgr.Map(colorizer))
	}

	lgr.SetupStdLogger(logOpts...)
	lgr.Setup(logOpts...)
}

func catchSignal() {
	// catch SIGQUIT and print stack traces
	sigChan := make(chan os.Signal, 1)
	go func() {
		for range sigChan {
			log.Print("[INFO] SIGQUIT detected")
			stacktrace := make([]byte, 8192)
			length := runtime.Stack(stacktrace, true)
			if length > 8192 {
				length = 8192
			}
			fmt.Println(string(stacktrace[:length]))
		}
	}()
	signal.Notify(sigChan, syscall.SIGQUIT)
}
