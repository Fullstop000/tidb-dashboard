// Copyright 2020 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

// @title Dashboard API
// @version 1.0
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @BasePath /dashboard/api
// @securityDefinitions.apikey JwtAuth
// @in header
// @name Authorization

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof" //nolint:gosec
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"go.etcd.io/etcd/pkg/transport"

	"github.com/joho/godotenv"
	"github.com/pingcap/log"
	flag "github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/pingcap-incubator/tidb-dashboard/pkg/apiserver"
	"github.com/pingcap-incubator/tidb-dashboard/pkg/config"
	"github.com/pingcap-incubator/tidb-dashboard/pkg/dbstore"
	http2 "github.com/pingcap-incubator/tidb-dashboard/pkg/http"
	"github.com/pingcap-incubator/tidb-dashboard/pkg/keyvisual"
	keyvisualinput "github.com/pingcap-incubator/tidb-dashboard/pkg/keyvisual/input"
	keyvisualregion "github.com/pingcap-incubator/tidb-dashboard/pkg/keyvisual/region"
	"github.com/pingcap-incubator/tidb-dashboard/pkg/pd"
	"github.com/pingcap-incubator/tidb-dashboard/pkg/swaggerserver"
	"github.com/pingcap-incubator/tidb-dashboard/pkg/tidb"
	"github.com/pingcap-incubator/tidb-dashboard/pkg/uiserver"
	"github.com/pingcap-incubator/tidb-dashboard/pkg/utils"
)

type DashboardCLIConfig struct {
	ListenHost     string
	ListenPort     int
	EnableDebugLog bool
	CoreConfig     *config.Config
	// key-visual file mode for debug
	KVFileStartTime int64
	KVFileEndTime   int64
}

// NewCLIConfig generates the configuration of the dashboard in standalone mode.
func NewCLIConfig() *DashboardCLIConfig {
	cfg := &DashboardCLIConfig{}
	cfg.CoreConfig = &config.Config{}

	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "Print version information and exit")
	flag.BoolVar(&showVersion, "version", false, "Print version information and exit")
	flag.StringVar(&cfg.ListenHost, "host", "0.0.0.0", "The listen address of the Dashboard Server")
	flag.IntVar(&cfg.ListenPort, "port", 12333, "The listen port of the Dashboard Server")
	flag.StringVar(&cfg.CoreConfig.DataDir, "data-dir", "/tmp/dashboard-data", "Path to the Dashboard Server data directory")
	flag.StringVar(&cfg.CoreConfig.PDEndPoint, "pd", "http://127.0.0.1:2379", "The PD endpoint that Dashboard Server connects to")
	flag.BoolVar(&cfg.EnableDebugLog, "debug", false, "Enable debug logs")
	// debug for keyvisual，hide help information
	flag.Int64Var(&cfg.KVFileStartTime, "keyviz-file-start", 0, "(debug) start time for file range in file mode")
	flag.Int64Var(&cfg.KVFileEndTime, "keyviz-file-end", 0, "(debug) end time for file range in file mode")

	caPath := flag.String("cluster-ca", "", "path of file that contains list of trusted SSL CAs.")
	certPath := flag.String("cluster-cert", "", "path of file that contains X509 certificate in PEM format..")
	keyPath := flag.String("cluster-key", "", "path of file that contains X509 key in PEM format.")

	_ = flag.CommandLine.MarkHidden("keyviz-file-start")
	_ = flag.CommandLine.MarkHidden("keyviz-file-end")

	flag.Parse()

	// setup TLSConfig
	if len(*caPath) != 0 && len(*certPath) != 0 && len(*keyPath) != 0 {
		tlsInfo := transport.TLSInfo{
			CertFile:      *certPath,
			KeyFile:       *keyPath,
			TrustedCAFile: *caPath,
		}
		tlsConfig, err := tlsInfo.ClientConfig()
		if err != nil {
			log.Fatal("Failed to load certificates", zap.Error(err))
		}
		cfg.CoreConfig.TLSConfig = tlsConfig
	}

	// normalize PDEndPoint
	if !strings.HasPrefix(cfg.CoreConfig.PDEndPoint, "http") {
		cfg.CoreConfig.PDEndPoint = fmt.Sprintf("http://%s", cfg.CoreConfig.PDEndPoint)
	}
	pdEndPoint, err := url.Parse(cfg.CoreConfig.PDEndPoint)
	if err != nil {
		log.Fatal("Invalid PD Endpoint", zap.Error(err))
	}
	pdEndPoint.Scheme = "http"
	if cfg.CoreConfig.TLSConfig != nil {
		pdEndPoint.Scheme = "https"
	}
	cfg.CoreConfig.PDEndPoint = pdEndPoint.String()

	if showVersion {
		utils.PrintInfo()
		exit(0)
	}

	// keyvisual
	startTime := cfg.KVFileStartTime
	endTime := cfg.KVFileEndTime
	if startTime != 0 || endTime != 0 {
		// file mode (debug)
		if startTime == 0 || endTime == 0 || startTime >= endTime {
			panic("keyviz-file-start must be smaller than keyviz-file-end, and none of them are 0")
		}
	}

	return cfg
}

func getContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sc := make(chan os.Signal, 1)
		signal.Notify(sc,
			syscall.SIGHUP,
			syscall.SIGINT,
			syscall.SIGTERM,
			syscall.SIGQUIT)
		<-sc
		cancel()
	}()
	return ctx
}

func main() {
	_ = godotenv.Load()

	// Flushing any buffered log entries
	defer log.Sync() //nolint:errcheck

	cliConfig := NewCLIConfig()
	ctx := getContext()

	store := dbstore.MustOpenDBStore(cliConfig.CoreConfig)
	defer store.Close() //nolint:errcheck

	etcdProvider, err := pd.NewLocalEtcdClientProvider(cliConfig.CoreConfig)
	if err != nil {
		_ = store.Close()
		log.Fatal("Cannot create etcd client", zap.Error(err))
	}

	tidbForwarder := tidb.NewForwarder(tidb.NewForwarderConfig(cliConfig.CoreConfig.TLSConfig), etcdProvider)
	// FIXME: Handle open error
	tidbForwarder.Open()        //nolint:errcheck
	defer tidbForwarder.Close() //nolint:errcheck

	httpClient := http2.NewHTTPClientWithConf(cliConfig.CoreConfig)

	// key visual
	remoteDataProvider := &keyvisualregion.PDDataProvider{
		FileStartTime:  cliConfig.KVFileStartTime,
		FileEndTime:    cliConfig.KVFileEndTime,
		PeriodicGetter: keyvisualinput.NewAPIPeriodicGetter(cliConfig.CoreConfig.PDEndPoint, httpClient),
		EtcdProvider:   etcdProvider,
		Store:          store,
	}
	keyvisualService := keyvisual.NewService(ctx, cliConfig.CoreConfig, remoteDataProvider, httpClient)
	keyvisualService.Start()
	defer keyvisualService.Close()

	services := &apiserver.Services{
		Store:         store,
		KeyVisual:     keyvisualService,
		TiDBForwarder: tidbForwarder,
		EtcdProvider:  etcdProvider,
		HTTPClient:    httpClient,
	}
	mux := http.DefaultServeMux
	mux.Handle("/dashboard/", http.StripPrefix("/dashboard", uiserver.Handler()))
	mux.Handle("/dashboard/api/", apiserver.Handler("/dashboard/api", cliConfig.CoreConfig, services))
	mux.Handle("/dashboard/api/swagger/", swaggerserver.Handler())

	listenAddr := fmt.Sprintf("%s:%d", cliConfig.ListenHost, cliConfig.ListenPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		_ = store.Close()
		log.Fatal("Dashboard server listen failed", zap.String("addr", listenAddr), zap.Error(err))
	}

	utils.LogInfo()
	if cliConfig.EnableDebugLog {
		log.SetLevel(zapcore.DebugLevel)
	}
	log.Info(fmt.Sprintf("Dashboard server is listening at %s", listenAddr))
	log.Info(fmt.Sprintf("UI:      http://127.0.0.1:%d/dashboard/", cliConfig.ListenPort))
	log.Info(fmt.Sprintf("API:     http://127.0.0.1:%d/dashboard/api/", cliConfig.ListenPort))
	log.Info(fmt.Sprintf("Swagger: http://127.0.0.1:%d/dashboard/api/swagger/", cliConfig.ListenPort))

	srv := &http.Server{Handler: mux}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		if err := srv.Serve(listener); err != http.ErrServerClosed {
			log.Fatal("Can not run server", zap.Error(err))
		}
		wg.Done()
	}()

	<-ctx.Done()
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Fatal("Can not stop server", zap.Error(err))
	}
	wg.Wait()
	log.Info("Stop dashboard server")
}

func exit(code int) {
	_ = log.Sync()
	os.Exit(code)
}
