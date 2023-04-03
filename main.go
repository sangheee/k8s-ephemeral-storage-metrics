package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

var (
	listenAddress        string
	scrapeIntervalSecond int64
	metricsPath          string
	verbosityLogLevel    string
)

func main() {
	flag.Int64Var(&scrapeIntervalSecond, "scrape-interval", int64FromEnv("SCRAPE_INTERVAL_SECOND", 15), "Metrics scraping interval")
	flag.StringVar(&listenAddress, "listen-address", ":9100", "Address on which to expose metrics and web interface.")
	flag.StringVar(&metricsPath, "metrics-path", "/metrics", "Path under which to expose metrics.")
	flag.StringVar(&verbosityLogLevel, "log.verbosity", "0", "Verbosity log level")

	flag.Parse()

	klog.InitFlags(flag.CommandLine)
	err := flag.Set("v", verbosityLogLevel)
	if err != nil {
		klog.Errorf("error on setting v to %s: %v", verbosityLogLevel, err)
	}
	defer klog.Flush()

	klog.Info("Starting ephemeral-storage-exporter")
	// Use the clientcmd library to load the Kubernetes client configuration
	cfg, err := config.GetConfig()
	if err != nil {
		panic(fmt.Errorf("failed to create Kubernetes client config: %v", err))
	}
	// create the clientset
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		panic(err.Error())
	}

	manager := NewManager(clientset, time.Duration(scrapeIntervalSecond)*time.Second)
	// Start the manager.
	if err := manager.Start(); err != nil {
		klog.Fatalf("Failed to start manager: %v", err)
	}
	defer func() {
		if err := manager.Stop(); err != nil {
			klog.Errorf("Failed to stop container manager: %v", err)
		}
	}()

	prometheus.MustRegister(newEphemeralStorageCollector(manager))
	http.Handle(metricsPath, promhttp.Handler())

	srv := &http.Server{Addr: listenAddress}
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received.
	go func() {
		sig := <-stopCh
		klog.Infof("Exiting given signal: %v", sig)
		if err := srv.Shutdown(context.Background()); err != nil {
			klog.ErrorS(err, "failed to shutdown server")
		}
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		klog.ErrorS(err, "error starting HTTP server")
	}
}

func int64FromEnv(env string, defaultValue int64) int64 {
	str, ok := os.LookupEnv(env)
	if !ok {
		return defaultValue
	}

	num, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return defaultValue
	}
	return num
}
