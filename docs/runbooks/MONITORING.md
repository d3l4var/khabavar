# Runbook: Monitoring & Observability (R-18 §۶)

آخرین به‌روزرسانی: 2026-11-26

## هدف

این ران‌بوک پیکربندی monitoring khabavar را توضیح می‌دهد — متریک‌ها، هشدارها، داشبوردها، و لاگ‌سازی ساختاریافته.

## معماری مشاهده‌پذیری

```
khabavar (slog JSON) → journald → Loki
khabavar (/metrics)  → Prometheus → AlertManager → Telegram
                     ↓
                   Grafana (dashboards)
```

## متریک‌ها (Prometheus)

### اندپوینت متریک

هر unit یک `/metrics` endpoint دارد (در کنار `/healthz`). متریک‌ها با فرمت Prometheus exposition:

```
# HELP khabavar_http_requests_total Total HTTP requests by method/path/status.
# TYPE khabavar_http_requests_total counter
khabavar_http_requests_total{method="GET",path="/api/v1/products/{id}",status="200"} 1234

# HELP khabavar_http_request_duration_seconds Request latency.
# TYPE khabavar_http_request_duration_seconds histogram
khabavar_http_request_duration_seconds_bucket{path="...",le="0.1"} 1200

# HELP khabavar_outbox_dispatched_total Outbox events dispatched.
# TYPE khabavar_outbox_dispatched_total counter
khabavar_outbox_dispatched_total{event_type="OrderPaid",status="ok"} 56

# HELP khabavar_db_connections_active Active DB connections.
# TYPE khabavar_db_connections_active gauge
khabavar_db_connections_active 12
```

### متریک‌های کلیدی (Golden Signals)

| سیگنال | متریک | هشدار |
|--------|-------|-------|
| **Traffic** | `rate(http_requests_total[5m])` | — |
| **Errors** | `rate(http_requests_total{status=~"5.."}[5m])` | > 0.1 → S2 |
| **Latency** | `histogram_quantile(0.95, http_request_duration_seconds)` | p95 > 2s → S3 |
| **Saturation** | `db_connections_active / db_connections_max` | > 0.8 → S3 |

## هشدارها (AlertManager)

### قواعد هشدار (`prometheus-rules.yml`)

```yaml
groups:
  - name: khabavar
    rules:
      # S1: API down
      - alert: KhabavarApiDown
        expr: up{khabavar-api} == 0
        for: 1m
        labels: { severity: S1 }
        annotations: { summary: "khabavar-api is down" }

      # S2: high 5xx rate
      - alert: KhabavarHighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.1
        for: 2m
        labels: { severity: S2 }
        annotations: { summary: "API error rate > 0.1/s" }

      # S3: high DB connection usage
      - alert: KhabavarDBConnectionsHigh
        expr: db_connections_active / db_connections_max > 0.8
        for: 5m
        labels: { severity: S3 }

      # S3: outbox backlog growing
      - alert: KhabavarOutboxBacklog
        expr: khabavar_outbox_pending > 100
        for: 10m
        labels: { severity: S3 }
        annotations: { summary: "Outbox backlog > 100 events" }

      # S2: OTP rate limit triggered (possible attack)
      - alert: KhabavarOTPAbuse
        expr: rate(identity_otp_rate_limited_total[10m]) > 5
        for: 5m
        labels: { severity: S2 }
        annotations: { summary: "OTP rate limit triggered > 5 times in 10m" }
```

### مسیر هشدار

- **S1/S2**: Telegram bot → کانال `#khabavar-alerts` + PagerDuty
- **S3/S4**: Telegram only (صف روزانه)

## داشبوردها (Grafana)

### داشبورد اصلی (khabavar-overview)

پنل‌ها:
1. **Request rate** by status code (۴ رنگ: 2xx/3xx/4xx/5xx)
2. **Latency p50/p95/p99** by path
3. **Active DB connections** (gauge با thresholds)
4. **Outbox: dispatched vs pending** (time series)
5. **OTP: success vs rate-limited** (time series)
6. **Orders: created/paid/cancelled** (استاپед by hour)
7. **Error log rate** (از Loki)

### داشبورد تجاری (khabavar-commerce)

1. **Orders per day** (bar)
2. **Revenue per day** (line, IRR)
3. **Payment success rate** (gauge)
4. **Refund volume** (bar)
5. **Top products by sales** (table)

## لاگ‌سازی (Loki)

### فرمت لاگ

khabavar از `slog` با JSON handler استفاده می‌کند. هر خط:

```json
{
  "time": "2026-11-26T14:30:00Z",
  "level": "INFO",
  "msg": "http listen",
  "addr": ":8080",
  "unit": "api"
}
```

### کوئری‌های مفید (LogQL)

```logql
# خطاهای ۵xx در آخرین ۵ دقیقه
{unit="api"} |= "ERROR" | json | level >= "ERROR"

# لاگ‌های یک سفارش خاص
{unit="api"} |= "ORD-1001"

# OTP‌های rate-limited
{unit="identity"} |= "rate_limited"
```

## Uptime Monitoring

یک uptime monitor خارجی (UptimeRobot/BetterStack) هر ۳۰ ثانیه `https://khabavar.ir/healthz` را چک می‌کند. اگر ۲ پشت‌سرهم fail → هشدار S1.

## بررسی روزانه (Daily Check)

هر صبح ops چک می‌کند:
- [ ] داشبورد اصلی سبز است (هیچ هشدار فعال)
- [ ] backup شب گذشته موفق (`/var/log/khabavar-backup.log`)
- [ ] outbox pending = 0
- [ ] DB connections < 50% max
- [ ] لاگ خطا در ۲۴ ساعت گذشته بررسی شد
