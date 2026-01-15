# Deployment Guide

This guide covers deploying Apiary in different configurations: single-node, distributed, and Kubernetes.

## Single-Node Deployment

Single-node deployment uses embedded NATS and BadgerDB. This is the simplest configuration and suitable for development, testing, and small production workloads.

### Prerequisites

- Go 1.21 or later
- 2GB+ RAM
- 10GB+ disk space

### Installation

1. **Build binaries:**

```bash
make build
```

2. **Create data directory:**

```bash
sudo mkdir -p /var/apiary
sudo chown $USER:$USER /var/apiary
```

3. **Start apiaryd:**

```bash
./bin/apiaryd -data-dir /var/apiary -port 8080
```

### Configuration

Default configuration:
- **Data directory**: `/var/apiary`
- **API port**: `8080`
- **Embedded NATS**: Yes
- **BadgerDB**: Embedded in data directory

### Health Checks

```bash
# Liveness probe
curl http://localhost:8080/healthz

# Readiness probe
curl http://localhost:8080/ready
```

### Service Management (systemd)

Create `/etc/systemd/system/apiaryd.service`:

```ini
[Unit]
Description=Apiary Orchestrator
After=network.target

[Service]
Type=simple
User=apiary
Group=apiary
ExecStart=/usr/local/bin/apiaryd -data-dir /var/apiary -port 8080
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl enable apiaryd
sudo systemctl start apiaryd
sudo systemctl status apiaryd
```

## Distributed Deployment

Distributed deployment uses external NATS, Redis (optional), and etcd for leader election. This configuration supports horizontal scaling and high availability.

### Architecture

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Queen 1   │    │   Queen 2   │    │   Queen 3   │
│  (Leader)   │    │ (Follower)  │    │ (Follower)  │
└──────┬──────┘    └──────┬──────┘    └──────┬──────┘
       │                  │                  │
       └──────────────────┼──────────────────┘
                          │
              ┌───────────┴───────────┐
              │                       │
       ┌──────▼──────┐       ┌───────▼────────┐
       │   etcd      │       │  NATS Cluster  │
       │   Cluster   │       │   (3 nodes)    │
       └─────────────┘       └────────────────┘
```

### Prerequisites

- External NATS cluster (3+ nodes recommended)
- etcd cluster (3+ nodes for HA)
- Redis (optional, for shared state)

### NATS Setup

1. **Install NATS:**

```bash
# Download NATS server
wget https://github.com/nats-io/nats-server/releases/download/v2.10.7/nats-server-v2.10.7-linux-amd64.zip
unzip nats-server-v2.10.7-linux-amd64.zip
sudo mv nats-server-v2.10.7-linux-amd64/nats-server /usr/local/bin/
```

2. **Start NATS cluster:**

```bash
# Node 1
nats-server -p 4222 -cluster nats://localhost:6222 -routes nats://localhost:6223,nats://localhost:6224

# Node 2
nats-server -p 4223 -cluster nats://localhost:6223 -routes nats://localhost:6222,nats://localhost:6224

# Node 3
nats-server -p 4224 -cluster nats://localhost:6224 -routes nats://localhost:6222,nats://localhost:6223
```

3. **Enable JetStream (for persistence):**

```bash
nats-server -p 4222 -js -sd /var/nats/data -cluster nats://localhost:6222
```

### etcd Setup

1. **Install etcd:**

```bash
ETCD_VER=v3.5.9
wget https://github.com/etcd-io/etcd/releases/download/${ETCD_VER}/etcd-${ETCD_VER}-linux-amd64.tar.gz
tar xzvf etcd-${ETCD_VER}-linux-amd64.tar.gz
sudo mv etcd-${ETCD_VER}-linux-amd64/etcd* /usr/local/bin/
```

2. **Start etcd cluster:**

```bash
# Node 1
etcd --name node1 \
  --data-dir /var/etcd/node1 \
  --listen-client-urls http://0.0.0.0:2379 \
  --advertise-client-urls http://localhost:2379 \
  --listen-peer-urls http://localhost:2380 \
  --initial-advertise-peer-urls http://localhost:2380 \
  --initial-cluster node1=http://localhost:2380,node2=http://localhost:2381,node3=http://localhost:2382 \
  --initial-cluster-token apiary-etcd-cluster \
  --initial-cluster-state new

# Node 2
etcd --name node2 \
  --data-dir /var/etcd/node2 \
  --listen-client-urls http://0.0.0.0:2379 \
  --advertise-client-urls http://localhost:2379 \
  --listen-peer-urls http://localhost:2381 \
  --initial-advertise-peer-urls http://localhost:2381 \
  --initial-cluster node1=http://localhost:2380,node2=http://localhost:2381,node3=http://localhost:2382 \
  --initial-cluster-token apiary-etcd-cluster \
  --initial-cluster-state new

# Node 3 (similar pattern)
```

### Running Distributed Apiary

1. **Configure apiaryd to use external services:**

```bash
# Set environment variables
export APIARY_NATS_URLS="nats://nats1:4222,nats://nats2:4222,nats://nats3:4222"
export APIARY_ETCD_ENDPOINTS="http://etcd1:2379,http://etcd2:2379,http://etcd3:2379"
export APIARY_LEADER_KEY="/apiary/leader"
export APIARY_LEADER_LEASE_TTL=30

# Start apiaryd
./bin/apiaryd -data-dir /var/apiary -port 8080
```

2. **Verify leader election:**

```bash
# Check which Queen is the leader
curl http://queen1:8080/ready
curl http://queen2:8080/ready
curl http://queen3:8080/ready
```

### Configuration Options

Environment variables for distributed deployment:

- `APIARY_NATS_URLS`: Comma-separated NATS server URLs
- `APIARY_ETCD_ENDPOINTS`: Comma-separated etcd endpoints
- `APIARY_LEADER_KEY`: etcd key for leader election (default: `/apiary/leader`)
- `APIARY_LEADER_LEASE_TTL`: Leader lease TTL in seconds (default: 30)

## Kubernetes Deployment

### Prerequisites

- Kubernetes cluster (1.21+)
- kubectl configured
- Persistent volumes for data storage

### Namespace

Create a namespace:

```bash
kubectl create namespace apiary
```

### ConfigMap

Create `k8s/configmap.yaml`:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: apiary-config
  namespace: apiary
data:
  data-dir: "/var/apiary"
  port: "8080"
  nats-urls: "nats://nats-service:4222"
  etcd-endpoints: "http://etcd-service:2379"
```

### StatefulSet

Create `k8s/statefulset.yaml`:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: apiaryd
  namespace: apiary
spec:
  serviceName: apiaryd
  replicas: 3
  selector:
    matchLabels:
      app: apiaryd
  template:
    metadata:
      labels:
        app: apiaryd
    spec:
      containers:
      - name: apiaryd
        image: apiary/apiaryd:latest
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: APIARY_NATS_URLS
          valueFrom:
            configMapKeyRef:
              name: apiary-config
              key: nats-urls
        - name: APIARY_ETCD_ENDPOINTS
          valueFrom:
            configMapKeyRef:
              name: apiary-config
              key: etcd-endpoints
        volumeMounts:
        - name: data
          mountPath: /var/apiary
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 10Gi
```

### Service

Create `k8s/service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: apiaryd
  namespace: apiary
spec:
  type: LoadBalancer
  ports:
  - port: 8080
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: apiaryd
```

### Deploy

```bash
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/statefulset.yaml
kubectl apply -f k8s/service.yaml

# Check status
kubectl get pods -n apiary
kubectl logs -n apiary -l app=apiaryd

# Port forward for local access
kubectl port-forward -n apiary svc/apiaryd 8080:8080
```

### External NATS in Kubernetes

Deploy NATS using the official Helm chart:

```bash
helm repo add nats https://nats-io.github.io/k8s/helm/charts/
helm install nats nats/nats \
  --namespace nats \
  --create-namespace \
  --set nats.jetstream.enabled=true
```

Then update the ConfigMap to use the NATS service:

```yaml
nats-urls: "nats://nats.nats.svc.cluster.local:4222"
```

### External etcd in Kubernetes

Deploy etcd using the official etcd operator or Helm chart:

```bash
# Using etcd operator (if available)
# Or use a managed etcd service
```

Update ConfigMap:

```yaml
etcd-endpoints: "http://etcd-client.etcd.svc.cluster.local:2379"
```

## Monitoring

### Metrics

Apiary exposes OpenTelemetry metrics. Configure your observability backend to scrape metrics.

### Logging

Logs are structured JSON. Use a log aggregation system (ELK, Loki, etc.) to collect and analyze logs.

### Health Checks

Monitor health check endpoints:

```bash
# Liveness (is the process running?)
curl http://localhost:8080/healthz

# Readiness (can it serve traffic?)
curl http://localhost:8080/ready
```

## Backup and Recovery

### BadgerDB Backup

BadgerDB data is stored in the data directory. To backup:

```bash
# Stop apiaryd
sudo systemctl stop apiaryd

# Backup data directory
tar -czf apiary-backup-$(date +%Y%m%d).tar.gz /var/apiary

# Restart apiaryd
sudo systemctl start apiaryd
```

### Recovery

```bash
# Stop apiaryd
sudo systemctl stop apiaryd

# Restore data
tar -xzf apiary-backup-YYYYMMDD.tar.gz -C /

# Restart apiaryd
sudo systemctl start apiaryd
```

## Troubleshooting

### API Server Not Starting

- Check if port 8080 is available: `netstat -tuln | grep 8080`
- Check logs: `journalctl -u apiaryd -f`
- Verify data directory permissions

### Leader Election Issues

- Verify etcd connectivity: `etcdctl endpoint health`
- Check etcd cluster status: `etcdctl member list`
- Verify leader key: `etcdctl get /apiary/leader`

### NATS Connection Issues

- Verify NATS is running: `nats-server -v`
- Check NATS cluster status
- Verify network connectivity between Apiary and NATS

### Performance Issues

- Check resource usage: `top`, `htop`
- Review benchmark results (see `docs/README.md` for benchmark instructions)
- Profile components (see `build-docs/profiling.md`)
