# customapi Deep Dive — Flexible Widget for Any JSON API
#
# The customapi widget is the most powerful widget in MyServer.
# It can display data from ANY JSON endpoint, local or remote.
#
# Key features:
#   - Dot-separated field path extraction (nested JSON)
#   - Formatting: number, bytes, duration, percent, date
#   - Display modes: text, dynamic-list, graph
#   - Supports file:// scheme for local JSON
#   - Supports env var substitution in URLs

---
# ═══════════════════════════════════════════════════════════════════════════════
# DISPLAY MODE: text (default)
#
# Extracts a single value and formats it.
# ═══════════════════════════════════════════════════════════════════════════════

# Example: CPU usage from a metrics endpoint
widget:
  type: customapi
  url: http://localhost:8080/api/metrics
  display: text
  mappings:
    field: cpu.usage
    format: percent

# Example: Memory usage in bytes
widget:
  type: customapi
  url: http://localhost:8080/api/metrics
  display: text
  mappings:
    field: memory.used
    format: bytes

# Example: Active users (plain number)
widget:
  type: customapi
  url: https://analytics.example.com/api/stats
  display: text
  mappings:
    field: active_users
    format: number

# Example: Uptime as duration
widget:
  type: customapi
  url: http://localhost:8080/api/status
  display: text
  mappings:
    field: uptime_seconds
    format: duration

# Example: Last backup date
widget:
  type: customapi
  url: http://localhost:8080/api/backups
  display: text
  mappings:
    field: last_backup.date
    format: date

# Example: No format (raw string)
widget:
  type: customapi
  url: http://localhost:8080/api/version
  display: text
  mappings:
    field: version

---
# ═══════════════════════════════════════════════════════════════════════════════
# DISPLAY MODE: dynamic-list
#
# Renders a list of items. Each item has a name, label, and link target.
# The handler extracts items from the JSON response using the mappings.
# ═══════════════════════════════════════════════════════════════════════════════

# Required JSON structure:
# {
#   "items_key": [
#     {"name_key": "Item 1", "label_key": "v1.0", "target_key": "https://..."},
#     {"name_key": "Item 2", "label_key": "v2.0", "target_key": "https://..."}
#   ]
# }

# Example: Releases list
widget:
  type: customapi
  url: https://api.github.com/repos/owner/repo/releases
  display: dynamic-list
  mappings:
    items: releases          # top-level key containing the array
    name: name               # item field for the link text
    label: tag_name          # item field for the subtitle
    target: html_url         # item field for the link href

# Example: Internal demo catalog (local JSON)
widget:
  type: customapi
  url: file://data/demos.json
  display: dynamic-list
  mappings:
    items: demos
    name: title
    label: version
    target: url

# Example: Queue items
widget:
  type: customapi
  url: http://localhost:8080/api/queue
  display: dynamic-list
  mappings:
    items: queue
    name: job_name
    label: status
    target: details_url

# Example: Alerts/notifications
widget:
  type: customapi
  url: http://localhost:8080/api/alerts
  display: dynamic-list
  mappings:
    items: alerts
    name: summary
    label: severity
    target: link

# Example: Top-level array (items key omitted, use top-level array)
# JSON: [{"name": "A", "label": "v1", "target": "..."}, ...]
widget:
  type: customapi
  url: http://localhost:8080/api/list
  display: dynamic-list
  mappings:
    items: ""                # empty string = top-level array
    name: name
    label: version
    target: url

---
# ═══════════════════════════════════════════════════════════════════════════════
# DISPLAY MODE: graph
#
# Extracts label/value pairs for charting.
# Returns {labels: [...], values: [...]} to the frontend.
# ═══════════════════════════════════════════════════════════════════════════════

# Required JSON structure:
# {
#   "items_key": [
#     {"name_key": "Label 1", "value_key": 10},
#     {"name_key": "Label 2", "value_key": 20}
#   ]
# }

# Example: Request volume by endpoint
widget:
  type: customapi
  url: http://localhost:8080/api/metrics
  display: graph
  mappings:
    items: data
    name: endpoint
    value: count

# Example: Disk usage by mount
widget:
  type: customapi
  url: http://localhost:8080/api/disk
  display: graph
  mappings:
    items: mounts
    name: path
    value: used_percent

# Example: CPU usage by core
widget:
  type: customapi
  url: http://localhost:8080/api/cpu
  display: graph
  mappings:
    items: cores
    name: core_id
    value: usage_percent

---
# ═══════════════════════════════════════════════════════════════════════════════
# FIELD PATH SYNTAX
#
# The 'field' key in mappings supports dot-separated paths for nested JSON.
# ═══════════════════════════════════════════════════════════════════════════════

# Simple field:
#   JSON: {"version": "1.2.3"}
#   field: version

# Nested object:
#   JSON: {"system": {"cpu": {"usage": 45.2}}}
#   field: system.cpu.usage

# Array index:
#   JSON: {"results": [{"name": "First"}, {"name": "Second"}]}
#   field: results.0.name     # -> "First"

# Mixed:
#   JSON: {"data": {"disks": [{"mount": "/", "usage": 82}]}}
#   field: data.disks.0.usage  # -> 82

---
# ═══════════════════════════════════════════════════════════════════════════════
# FORMAT REFERENCE
#
# Format is applied to the extracted value before display.
# ═══════════════════════════════════════════════════════════════════════════════

# number    -> Integer or 2-decimal float (e.g. 42, 3.14)
# bytes     -> Human-readable bytes (e.g. 1.50 GiB, 256.00 MiB)
# duration  -> Go duration string (e.g. 2h30m0s)
# percent   -> Percentage with 1 decimal (e.g. 45.2%)
# date      -> RFC3339 to "2006-01-02 15:04" (e.g. 2024-01-15 09:30)
# default   -> Raw string via fmt.Sprint

---
# ═══════════════════════════════════════════════════════════════════════════════
# AUTHENTICATION
#
# customapi supports the same auth as all widgets:
# ═══════════════════════════════════════════════════════════════════════════════

# Basic auth:
widget:
  type: customapi
  url: http://localhost:8080/api/data
  username: admin
  password: secret
  display: text
  mappings:
    field: value

# API key in query:
widget:
  type: customapi
  url: http://localhost:8080/api/data?apikey={apiKey}
  apiKey: YOUR_KEY
  display: text
  mappings:
    field: value

# Bearer token in header:
widget:
  type: customapi
  url: http://localhost:8080/api/data
  key: YOUR_BEARER_TOKEN
  display: text
  mappings:
    field: value

# Custom headers:
widget:
  type: customapi
  url: http://localhost:8080/api/data
  headers:
    X-API-Key: YOUR_KEY
    X-Custom: value
  display: text
  mappings:
    field: value

---
# ═══════════════════════════════════════════════════════════════════════════════
# LOCAL JSON (file:// scheme)
#
# Reads JSON directly from config/ without HTTP round-trip.
# ═══════════════════════════════════════════════════════════════════════════════

# Relative path (resolves to /app/config/data/file.json):
widget:
  type: customapi
  url: file://data/metrics.json
  display: text
  mappings:
    field: cpu.usage

# The file config/data/metrics.json:
# {"cpu": {"usage": 45.2}, "memory": {"used": 8589934592}}

# Absolute path:
widget:
  type: customapi
  url: file:///app/config/data/metrics.json
  display: dynamic-list
  mappings:
    items: services
    name: name
    label: status
    target: url

---
# ═══════════════════════════════════════════════════════════════════════════════
# ENV VAR SUBSTITUTION
#
# URLs and widget values support env var substitution.
# ═══════════════════════════════════════════════════════════════════════════════

# Set HOMEPAGE_VAR_METRICS_URL=http://localhost:8080
widget:
  type: customapi
  url: "{{HOMEPAGE_VAR_METRICS_URL}}/api/stats"
  display: text
  mappings:
    field: active_users

# Set HOMEPAGE_VAR_API_KEY=secret123
widget:
  type: customapi
  url: http://localhost:8080/api/data
  key: "{{HOMEPAGE_VAR_API_KEY}}"
  display: text
  mappings:
    field: value

---
# ═══════════════════════════════════════════════════════════════════════════════
# COMPLETE REAL-WORLD EXAMPLES
# ═══════════════════════════════════════════════════════════════════════════════

# 1. GitHub releases for a project
- Tools:
    - Releases:
        href: https://github.com/owner/repo/releases
        description: Latest releases
        icon: si:github
        widget:
          type: customapi
          url: https://api.github.com/repos/owner/repo/releases
          display: dynamic-list
          mappings:
            items: releases
            name: name
            label: tag_name
            target: html_url

# 2. Docker container count
- Infrastructure:
    - Docker:
        href: https://docker.example.com
        description: Container runtime
        icon: si:docker
        widget:
          type: customapi
          url: file://data/docker-stats.json
          display: text
          mappings:
            field: container_count
            format: number

# 3. System load average
- Monitoring:
    - Load:
        href: https://monitoring.example.com
        description: System load
        icon: mdi:gauge
        widget:
          type: customapi
          url: http://localhost:9100/api/v1/load
          display: text
          mappings:
            field: load_avg.1m
            format: number

# 4. Backup status list
- Administration:
    - Backups:
        href: https://backups.example.com
        description: Backup jobs
        icon: mdi:backup-restore
        widget:
          type: customapi
          url: file://data/backups.json
          display: dynamic-list
          mappings:
            items: jobs
            name: name
            label: last_status
            target: url

# 5. SSL certificate expiry
- Security:
    - Certificates:
        href: https://certs.example.com
        description: SSL cert status
        icon: mdi:certificate
        widget:
          type: customapi
          url: file://data/certs.json
          display: dynamic-list
          mappings:
            items: certificates
            name: domain
            label: days_remaining
            target: url

# 6. Active incidents
- Status:
    - Incidents:
        href: https://status.example.com
        description: Active incidents
        icon: mdi:alert-circle
        widget:
          type: customapi
          url: https://status.example.com/api/incidents
          display: dynamic-list
          mappings:
            items: incidents
            name: title
            label: severity
            target: url

# 7. Top processes by CPU
- Monitoring:
    - Top CPU:
        href: https://monitoring.example.com
        description: Top processes
        icon: mdi:cpu-64-bit
        widget:
          type: customapi
          url: http://localhost:8080/api/top
          display: dynamic-list
          mappings:
            items: processes
            name: command
            label: cpu_percent
            target: url

# 8. Monthly bandwidth usage (graph)
- Networking:
    - Bandwidth:
        href: https://monitoring.example.com
        description: Monthly usage
        icon: mdi:chart-bar
        widget:
          type: customapi
          url: file://data/bandwidth.json
          display: graph
          mappings:
            items: months
            name: month
            value: total_gb

# 9. Recent deployments
- DevOps:
    - Deployments:
        href: https://deploy.example.com
        description: Recent deploys
        icon: mdi:rocket-launch
        widget:
          type: customapi
          url: https://deploy.example.com/api/deployments
          display: dynamic-list
          mappings:
            items: deployments
            name: app
            label: version
            target: url

# 10. Storage usage by pool
- Infrastructure:
    - Storage:
        href: https://nas.example.com
        description: Pool usage
        icon: mdi:harddisk
        widget:
          type: customapi
          url: file://data/storage.json
          display: dynamic-list
          mappings:
            items: pools
            name: name
            label: used_percent
            target: url
