#!/bin/bash
#
# Entry point for the myserver container.
#
# Responsibilities:
#   1. Allow PUID/PGID overrides for hosts that need a different uid
#      (e.g. to match a host volume's owner).
#   2. Grant the 'myserver' user access to /var/run/docker.sock (if mounted)
#      by detecting the socket's GID and adding the user to a matching
#      group on the fly. This avoids hardcoding the host's docker group GID
#      which varies between distros (Debian/Ubuntu usually 999, RHEL/Fedora
#      varies).
#   3. Seed /app/config from /app/skeleton on first boot if the config
#      volume is empty so the dashboard works out of the box without
#      manual setup.
#   4. Drop privileges to the 'myserver' user via su-exec before exec'ing
#      the binary.
#
set -euo pipefail

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

# ---------------------------------------------------------------------------
# 1) PUID / PGID override
# ---------------------------------------------------------------------------
# Recreate the myserver user/group with the requested PUID/PGID if they
# differ from the build-time defaults. Alpine ships addgroup/adduser from
# BusyBox; useradd/groupadd are NOT available.
current_uid="$(id -u myserver 2>/dev/null || echo -1)"
current_gid="$(id -g myserver 2>/dev/null || echo -1)"

if [ "${PUID}" != "${current_uid}" ] || [ "${PGID}" != "${current_gid}" ]; then
    # Skip GID 0 (root) and GID 1 (bin) — never reuse system groups.
    if [ "${PGID}" -gt 1 ] && ! getent group "${PGID}" >/dev/null 2>&1; then
        addgroup -g "${PGID}" myserver 2>/dev/null || true
    fi
    if [ "${PUID}" -gt 1 ]; then
        deluser myserver 2>/dev/null || true
        adduser -u "${PUID}" -G "$(getent group "${PGID}" 2>/dev/null | cut -d: -f1 || echo myserver)" -D -H myserver 2>/dev/null || true
    fi
fi

# ---------------------------------------------------------------------------
# 2) Docker socket access
# ---------------------------------------------------------------------------
# If the host Docker socket is bind-mounted, detect its GID and add the
# myserver user to a group with that GID so docker CLI and docker stats
# handlers can read it without running as root.
if [ -S /var/run/docker.sock ]; then
    sock_gid="$(stat -c '%g' /var/run/docker.sock 2>/dev/null || echo 0)"
    if [ "${sock_gid}" != "0" ]; then
        if ! getent group "${sock_gid}" >/dev/null 2>&1; then
            addgroup -g "${sock_gid}" docker-sock 2>/dev/null || true
        fi
        group_name="$(getent group "${sock_gid}" | cut -d: -f1)"
        if [ -n "${group_name}" ]; then
            addgroup myserver "${group_name}" 2>/dev/null || true
        fi
    fi
fi

# ---------------------------------------------------------------------------
# 3) Config seed on first boot
# ---------------------------------------------------------------------------
if [ -d /app/skeleton ] && [ -z "$(ls -A /app/config 2>/dev/null || true)" ]; then
    echo "[entrypoint] seeding /app/config from skeleton"
    cp -a /app/skeleton/. /app/config/
fi

# Ensure runtime dirs are writable by the myserver user.
chown -R myserver:myserver /app/config /app/scripts /app/data 2>/dev/null || true

# ---------------------------------------------------------------------------
# 4) Drop privileges and exec
# ---------------------------------------------------------------------------
# NOTE: use `su-exec myserver` (not `myserver:myserver`). The `user:group`
# form sets uid+gid explicitly and drops supplementary groups, which
# strips the docker-sock group added in step 2 and breaks Docker stats.
if [ "$(id -u)" = "0" ]; then
    exec su-exec myserver /app/myserver "$@"
fi

exec /app/myserver "$@"