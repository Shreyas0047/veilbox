#!/bin/sh
# Unix socket HTTP health endpoint
# Returns {"status":"ok","uptime":...} on connect
set -eu

SOCKET="${1:-/var/run/health.sock}"

# Remove existing socket if any
rm -f "$SOCKET"

# Parse uptime from /proc/uptime
uptime_seconds() {
    read -r sec _ < /proc/uptime 2>/dev/null
    sec="${sec:-0}"
    printf "%.0f" "$sec"
}

# HTTP response helper
http_response() {
    local status="$1" body="$2"
    printf "HTTP/1.1 %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s" \
        "$status" "${#body}" "$body"
}

# Listen loop (single connection, then exit — respawn from rcS loop or inetd)
while true; do
    # Listen on Unix socket with nc
    /usr/bin/nc -U -l -k "$SOCKET" 2>/dev/null | while read -r line; do
        case "$line" in
            GET*|POST*|HEAD*)
                uptime=$(uptime_seconds)
                body="{\"status\":\"ok\",\"uptime\":${uptime}}"
                http_response "200 OK" "$body"
                break
                ;;
            "")
                # End of headers, respond
                uptime=$(uptime_seconds)
                body="{\"status\":\"ok\",\"uptime\":${uptime}}"
                http_response "200 OK" "$body"
                break
                ;;
        esac
    done
done
