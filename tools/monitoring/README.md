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

## Sampling / Retention Defaults

- Prometheus `scrape_interval`: `15s` (`tools/monitoring/prometheus/prometheus.yml`)
- Prometheus `scrape_timeout`: `10s` (`tools/monitoring/prometheus/prometheus.yml`)
- Prometheus `evaluation_interval`: `15s` (`tools/monitoring/prometheus/prometheus.yml`)
- cAdvisor `housekeeping_interval`: `10s` (`tools/monitoring/docker-monitoring.services.yml`)
- Prometheus retention: `7d` (`PROMETHEUS_RETENTION` in `tools/monitoring/.env`)

## Change Procedure

1. Prometheus の取得/評価間隔を変える  
`tools/monitoring/prometheus/prometheus.yml` の `global` を編集します。
2. cAdvisor の収集間隔を変える  
`tools/monitoring/docker-monitoring.services.yml` の `--housekeeping_interval` を編集します。
3. 保持期間やポートを変える  
`tools/monitoring/.env` を編集します（例: `PROMETHEUS_RETENTION=14d`）。
4. 設定を反映する  
Prometheus 設定だけの変更なら reload:  
```bash
curl -X POST http://localhost:${PORT_PROMETHEUS:-9090}/-/reload
```  
Compose や `.env` 変更を含む場合は再作成:  
```bash
docker compose --env-file tools/monitoring/.env -f tools/monitoring/docker-monitoring.docker.yml up -d --force-recreate
docker compose --env-file tools/monitoring/.env -f tools/monitoring/docker-monitoring.containerd.yml up -d --force-recreate
```

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
