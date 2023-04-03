package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/context"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	stats "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

var namespace = "ephemeral_storage"

type manager struct {
	node                     string
	cli                      *kubernetes.Clientset
	scrapeInterval           time.Duration
	podEphemeralStorageStats []*podEphemeralStorageStat
	statsLastUpdatedTime     time.Time

	statsLock sync.Mutex
	wg        sync.WaitGroup
	stopCh    chan struct{}
	lock      sync.Mutex
	running   bool
}

type podEphemeralStorageStat struct {
	nodeName  string
	podName   string
	namespace string
	*stats.FsStats
}

func NewManager(cli *kubernetes.Clientset, interval time.Duration) *manager {
	currentNode, ok := os.LookupEnv("CURRENT_NODE_NAME")
	if !ok {
		klog.Warning("current node info is not passed.")
	}
	return &manager{
		node:           currentNode,
		cli:            cli,
		scrapeInterval: interval,
	}
}

func (m *manager) Start() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.running {
		return errors.New("ephemeral metrics manager is already running")
	}

	m.running = true
	m.stopCh = make(chan struct{})
	m.wg.Add(1)

	go func() {
		defer m.wg.Done()

		timer := time.NewTimer(0 * time.Second)
		defer timer.Stop()

		for {
			select {
			case <-m.stopCh:
			case <-timer.C:
			}
			start := time.Now()

			content, err := m.cli.RESTClient().Get().AbsPath(fmt.Sprintf("/api/v1/nodes/%s/proxy/stats/summary", m.node)).DoRaw(context.Background())
			if err != nil {
				klog.Error(err)
			}
			klog.V(4).Info("Fetched proxy stats from node : %s", m.node)

			raw := &stats.Summary{}
			_ = json.Unmarshal(content, &raw)

			nodeName := raw.Node.NodeName
			podEphemeralStorageStats := make([]*podEphemeralStorageStat, 0, len(raw.Pods))

			for _, podStat := range raw.Pods {
				// A pod that has just been created may not have a field below.
				podRef := podStat.PodRef
				ephemeralStorageStat := podStat.EphemeralStorage
				podEphemeralStorageStats = append(podEphemeralStorageStats, &podEphemeralStorageStat{
					namespace: podRef.Namespace,
					nodeName:  nodeName,
					podName:   podRef.Name,
					FsStats:   ephemeralStorageStat,
				})
			}

			func() {
				m.statsLock.Lock()
				defer m.statsLock.Unlock()

				m.podEphemeralStorageStats = podEphemeralStorageStats
			}()

			end := time.Now()
			duration := end.Sub(start)
			klog.V(3).Infof("Taking time to get node stat summary start:%v, end:%v, duration:%v", start, end, duration)

			timer.Reset(m.scrapeInterval - duration)
		}
	}()

	return nil
}

func (m *manager) Stop() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if !m.running {
		klog.Warning("metrics manager already stopped")
		return nil
	}

	defer func() {
		m.running = false
	}()

	close(m.stopCh)
	m.wg.Wait()
	return nil
}

func (m *manager) RecentStats() []podEphemeralStorageStat {
	m.statsLock.Lock()
	defer m.statsLock.Unlock()

	var ret []podEphemeralStorageStat
	for _, stat := range m.podEphemeralStorageStats {
		ret = append(ret, *stat)
	}
	return ret
}

type ephemeralStorageMetric struct {
	name        string
	help        string
	extraLabels []string
	valueType   prometheus.ValueType
	getValue    func(stats *stats.FsStats) float64
}

func (m *ephemeralStorageMetric) desc(baseLabels []string) *prometheus.Desc {
	return prometheus.NewDesc(m.name, m.help, append(baseLabels, m.extraLabels...), nil)
}

type ephemeralStorageCollector struct {
	nodeName string
	manager  *manager
	errors   prometheus.Gauge
	metrics  []*ephemeralStorageMetric
}

// https://github.com/kubernetes/kubernetes/blob/7d309e0104fedb57280b261e5677d919cb2a0e2d/staging/src/k8s.io/kubelet/pkg/apis/stats/v1alpha1/types.go#L128
// https://github.com/kubernetes/kubernetes/blob/7d309e0104fedb57280b261e5677d919cb2a0e2d/staging/src/k8s.io/kubelet/pkg/apis/stats/v1alpha1/types.go#L280-L305
func newEphemeralStorageCollector(manager *manager) *ephemeralStorageCollector {
	return &ephemeralStorageCollector{
		manager: manager,
		errors: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "scrape_error",
			Help:      "1 if there was an error while getting container metrics, 0 otherwise",
		}),
		metrics: []*ephemeralStorageMetric{
			{
				name:      "ephemeral_storage_pod_used_bytes",
				help:      "Used bytes to expose Ephemeral Storage metrics for pod",
				valueType: prometheus.GaugeValue,
				getValue: func(stats *stats.FsStats) float64 {
					return float64(*stats.UsedBytes)
				},
			},
			{
				name:      "ephemeral_storage_pod_available_bytes",
				help:      "Available bytes of ephemeral storage",
				valueType: prometheus.GaugeValue,
				getValue: func(stats *stats.FsStats) float64 {
					return float64(*stats.AvailableBytes)
				},
			},
			{
				name:      "ephemeral_storage_pod_capacity_bytes",
				help:      "Capacity bytes of pod ephemeral storage",
				valueType: prometheus.GaugeValue,
				getValue: func(stats *stats.FsStats) float64 {
					return float64(*stats.CapacityBytes)
				},
			},
		},
	}
}

// Collect implements prometheus.PrometheusCollector.
func (c *ephemeralStorageCollector) Collect(ch chan<- prometheus.Metric) {
	c.errors.Set(0)
	c.collectEphemeralStorageInfo(ch)
	c.errors.Collect(ch)
}

func (c *ephemeralStorageCollector) Describe(ch chan<- *prometheus.Desc) {
	c.errors.Describe(ch)
	for _, cm := range c.metrics {
		ch <- cm.desc([]string{})
	}
}

func (c *ephemeralStorageCollector) collectEphemeralStorageInfo(ch chan<- prometheus.Metric) {
	podEphemeralStorageStats := c.manager.RecentStats()
	for _, metric := range c.metrics {
		desc := metric.desc([]string{"node_name", "namespace_name", "pod_name"})
		for _, stat := range podEphemeralStorageStats {
			ch <- prometheus.MustNewConstMetric(desc, metric.valueType, metric.getValue(stat.FsStats), []string{stat.nodeName, stat.namespace, stat.podName}...)
		}
	}
}
