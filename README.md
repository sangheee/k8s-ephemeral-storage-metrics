# K8s Ephemeral Storage Metrics.


The goal of this project is to export ephemeral storage metric usage per pod to Prometheus that is address in this 
issue [Here](https://github.com/kubernetes/kubernetes/issues/69507)

Currently, this image is not being hosted and so you have to build it yourself at the moment. 

### Running

Building:

```bash
git clone https://github.com/sangheee/k8s-ephemeral-storage-metrics.git
cd k8s-ephemeral-storage-metrics
make
./ephemeral-storage-exporter <flags>
```

To see all available configuration flags:

```bash
./ephemeral-storage-exporter -h

Usage of ./ephemeral-storage-exporter:
  -kubeconfig string
        Paths to a kubeconfig. Only required if out-of-cluster.
  -listen-address string
        Address on which to expose metrics and web interface. (default ":9100")
  -log.verbosity string
        Verbosity log level (default "0")
  -metrics-path string
        Path under which to expose metrics. (default "/metrics")
  -scrape-interval int
        Metrics scraping interval (default 15)
```

Run binary:

```bash
CURRENT_NODE_NAME=${NODE_NAME} ./ephemeral-storage-exporter
```

Get metrics:

```bash
curl http://localhost:9100/metrics
```

### Metrics

All metrics (except golang and app metrics) are prefixed with **"ephemeral_storage_".**

**Always exported**

| metric       | description                                                           | 
|--------------|-----------------------------------------------------------------------|
| scrape_error | 1 if there was an error while getting container metrics, 0 otherwise. | 

**Ephemeral Storage Stats information**

Labels: `pod_name`, `naemspace_name`, `node_name`

| metric              | description                                             | 
|---------------------|---------------------------------------------------------|
| pod_used_bytes      | Used bytes to expose Ephemeral Storage metrics for pod. |
| pod_available_bytes | Available bytes of pod ephemeral storage.               |
| pod_capacity_bytes  | Capacity bytes of pod ephemeral storage.                |

