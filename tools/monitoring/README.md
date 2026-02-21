# Monitoring

`tools/monitoring` は ESB の監視スタック（cAdvisor / Prometheus / Grafana）を起動するための compose 一式です。

## Files

- `docker-monitoring.docker.yml`: Docker ランタイム向け
- `docker-monitoring.containerd.yml`: containerd ランタイム向け
- `docker-monitoring.services.yml`: 監視サービス定義本体
- `prometheus/prometheus.yml`: Prometheus 設定
- `grafana/`: Grafana の datasource / dashboard プロビジョニング

## Env

環境変数は `tools/monitoring/.env` を使用します。

## Start

Docker ランタイム:

```bash
docker compose --env-file tools/monitoring/.env -f tools/monitoring/docker-monitoring.docker.yml up -d
```

containerd ランタイム:

```bash
docker compose --env-file tools/monitoring/.env -f tools/monitoring/docker-monitoring.containerd.yml up -d
```

`--profile monitoring` は不要です。

## Stop

Docker ランタイム:

```bash
docker compose --env-file tools/monitoring/.env -f tools/monitoring/docker-monitoring.docker.yml down
```

containerd ランタイム:

```bash
docker compose --env-file tools/monitoring/.env -f tools/monitoring/docker-monitoring.containerd.yml down
```

## Access

- Grafana: `http://localhost:3000`
- Prometheus: `http://localhost:9090`
- cAdvisor: `http://localhost:8080`

Grafana の監視ダッシュボード:

- `http://localhost:3000/d/esb-mon/monitoring`
