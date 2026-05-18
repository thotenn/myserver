#!/bin/bash
# Script Templates for MyServer
#
# Place these in config/scripts/ and make them executable (chmod +x).
# Reference them in config/scripts.yaml by relative name.
#
# IMPORTANT RULES:
#   - Only .sh files are allowed
#   - Must be inside one of the configured scriptDirs
#   - Must NOT be world-writable (mode & 0o002 == 0)
#   - Inherits minimal env: PATH, HOME=/tmp, USER=myserver, SHELL=/bin/bash, TZ
#   - For Podman: set HOME and XDG_RUNTIME_DIR in scripts.yaml env:
#       env:
#         HOME: /home/tho
#         XDG_RUNTIME_DIR: /run/user/1000

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 1: Hello World
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

echo "Hello from MyServer scripts!"
echo "Current time: $(date)"
echo "User: $USER"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 2: Docker Container Restart
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

CONTAINER_NAME="${1:-nginx}"

echo "Restarting container: $CONTAINER_NAME"
docker restart "$CONTAINER_NAME"
echo "Container $CONTAINER_NAME restarted successfully"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 3: System Backup
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

BACKUP_DIR="/backup"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "Starting backup at $TIMESTAMP"

# Backup config
tar czf "$BACKUP_DIR/config_$TIMESTAMP.tar.gz" -C /app config

# Backup database (if available)
if command -v pg_dump >/dev/null 2>&1; then
    pg_dump -h localhost -U postgres mydb > "$BACKUP_DIR/db_$TIMESTAMP.sql"
    echo "Database backup complete"
fi

echo "Backup finished: $BACKUP_DIR/config_$TIMESTAMP.tar.gz"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 4: Docker Cleanup
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

echo "Cleaning Docker system..."

# Remove unused containers
docker container prune -f

# Remove unused images
docker image prune -af

# Remove unused volumes
docker volume prune -f

# Remove unused networks
docker network prune -f

echo "Docker cleanup complete"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 5: Health Check
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

SERVICES=("https://api.example.com/health" "https://db.example.com/health")
FAILED=0

for url in "${SERVICES[@]}"; do
    if curl -fsS "$url" >/dev/null 2>&1; then
        echo "OK: $url"
    else
        echo "FAIL: $url"
        FAILED=1
    fi
done

if [ "$FAILED" -eq 1 ]; then
    echo "Some health checks failed"
    exit 1
fi

echo "All health checks passed"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 6: Generate Local Metrics JSON
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

# Read CPU load (1-min average)
CPU=$(cat /proc/loadavg | awk '{print $1}')

# Read memory usage
MEM_INFO=$(cat /proc/meminfo)
MEM_TOTAL=$(echo "$MEM_INFO" | grep MemTotal | awk '{print $2}')
MEM_AVAILABLE=$(echo "$MEM_INFO" | grep MemAvailable | awk '{print $2}')
MEM_USED=$((MEM_TOTAL - MEM_AVAILABLE))
MEM_PERCENT=$(echo "scale=1; $MEM_USED * 100 / $MEM_TOTAL" | bc)

# Read disk usage
DISK_PERCENT=$(df / | tail -1 | awk '{print $5}' | sed 's/%//')

# Write JSON
cat > /app/config/data/metrics.json <<EOF
{
  "cpu": $CPU,
  "memory": $MEM_PERCENT,
  "disk": $DISK_PERCENT,
  "timestamp": "$(date -Iseconds)"
}
EOF

echo "Metrics written to /app/config/data/metrics.json"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 7: SSL Certificate Check
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

DOMAIN="${1:-example.com}"
DAYS_THRESHOLD=30

echo "Checking SSL certificate for $DOMAIN..."

EXPIRY=$(echo | openssl s_client -servername "$DOMAIN" -connect "$DOMAIN:443" 2>/dev/null | openssl x509 -noout -enddate | cut -d= -f2)
EXPIRY_EPOCH=$(date -d "$EXPIRY" +%s)
NOW_EPOCH=$(date +%s)
DAYS_REMAINING=$(( (EXPIRY_EPOCH - NOW_EPOCH) / 86400 ))

echo "Certificate expires in $DAYS_REMAINING days"

if [ "$DAYS_REMAINING" -lt "$DAYS_THRESHOLD" ]; then
    echo "WARNING: Certificate expires in less than $DAYS_THRESHOLD days!"
    exit 1
fi

echo "Certificate is valid"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 8: Update All Containers
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

echo "Pulling latest images..."
docker compose -f /opt/myserver/docker-compose.yml pull

echo "Recreating containers..."
docker compose -f /opt/myserver/docker-compose.yml up -d

echo "Pruning old images..."
docker image prune -af

echo "Update complete"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 9: Podman Container List
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

# NOTE: For Podman rootless, set in scripts.yaml:
#   env:
#     HOME: /home/tho
#     XDG_RUNTIME_DIR: /run/user/1000

echo "Podman containers:"
podman ps --format "table {{.Names}}\t{{.Status}}\t{{.Image}}"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 10: Database Backup
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

PGHOST="${PGHOST:-localhost}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-myapp}"
BACKUP_DIR="/backup"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "Backing up database $PGDATABASE..."
pg_dump -h "$PGHOST" -U "$PGUSER" "$PGDATABASE" | gzip > "$BACKUP_DIR/db_${PGDATABASE}_$TIMESTAMP.sql.gz"

echo "Backup complete: $BACKUP_DIR/db_${PGDATABASE}_$TIMESTAMP.sql.gz"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 11: Sync to S3
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
S3_BUCKET="${S3_BUCKET:-my-backups}"
SOURCE_DIR="${1:-/app/config}"

echo "Syncing $SOURCE_DIR to s3://$S3_BUCKET/"
aws s3 sync "$SOURCE_DIR" "s3://$S3_BUCKET/config/" --region "$AWS_REGION"

echo "Sync complete"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 12: Log Tail (last N lines)
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

LOG_FILE="${1:-/var/log/myapp/app.log}"
LINES="${2:-50}"

echo "Last $LINES lines of $LOG_FILE:"
tail -n "$LINES" "$LOG_FILE"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 13: Service Status Overview
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

echo "=== System Status ==="
echo "Load: $(cat /proc/loadavg | awk '{print $1, $2, $3}')"
echo "Uptime: $(uptime -p)"
echo ""
echo "=== Docker Containers ==="
docker ps --format "table {{.Names}}\t{{.Status}}" 2>/dev/null || echo "Docker not available"
echo ""
echo "=== Disk Usage ==="
df -h / /data 2>/dev/null || df -h /

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 14: Generate Service Status JSON
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

# Generate a JSON file for a customapi dynamic-list widget

cat > /app/config/data/services.json <<'EOF'
{
  "services": [
EOF

FIRST=true
for container in nginx postgres redis pihole; do
    if docker ps --format '{{.Names}}' | grep -q "^${container}$"; then
        STATUS="running"
    else
        STATUS="stopped"
    fi

    if [ "$FIRST" = true ]; then
        FIRST=false
    else
        echo "," >> /app/config/data/services.json
    fi

    cat >> /app/config/data/services.json <<EOF
    {"name": "$container", "status": "$STATUS", "url": "https://${container}.example.com"}
EOF
done

cat >> /app/config/data/services.json <<'EOF'
  ]
}
EOF

echo "Service status JSON generated"

# ═══════════════════════════════════════════════════════════════════════════════
# TEMPLATE 15: Rotate Logs
# ═══════════════════════════════════════════════════════════════════════════════
#!/bin/bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-/var/log/myapp}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"

echo "Rotating logs in $LOG_DIR..."

# Compress logs older than 1 day
find "$LOG_DIR" -name "*.log" -mtime +1 -exec gzip {} \;

# Delete compressed logs older than retention period
find "$LOG_DIR" -name "*.log.gz" -mtime +$RETENTION_DAYS -delete

echo "Log rotation complete"
