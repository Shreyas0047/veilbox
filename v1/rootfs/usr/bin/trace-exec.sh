#!/bin/sh
# Minimal eBPF execve tracer using bpftool
# Logs exec events to syslog via logger
set -eu

BPFTOOL=/usr/bin/bpftool
LOGGER=""

# Find logger (BusyBox provides /bin/logger)
for p in /bin/logger /usr/bin/logger /sbin/logger; do
    if [ -x "$p" ]; then
        LOGGER="$p"
        break
    fi
done

if [ ! -x "$BPFTOOL" ]; then
    echo "bpftool not found. Install it first." >&2
    exit 1
fi

case "${1:-}" in
    start)
        if [ -z "$LOGGER" ]; then
            echo "logger not found, falling back to direct write" >&2
            "$BPFTOOL" trace -p $$ 2>/dev/null >> /var/log/trace-exec.log &
        else
            "$BPFTOOL" trace -p $$ 2>/dev/null | while read -r line; do
                "$LOGGER" -t bpf-exec "$line"
            done &
        fi
        echo $! > /var/run/trace-exec.pid
        echo "trace-exec started (pid $(cat /var/run/trace-exec.pid))"
        ;;
    stop)
        if [ -f /var/run/trace-exec.pid ]; then
            kill "$(cat /var/run/trace-exec.pid)" 2>/dev/null || true
            /bin/rm -f /var/run/trace-exec.pid
            echo "trace-exec stopped"
        fi
        ;;
    status)
        if [ -f /var/run/trace-exec.pid ] && kill -0 "$(cat /var/run/trace-exec.pid)" 2>/dev/null; then
            echo "trace-exec running (pid $(cat /var/run/trace-exec.pid))"
        else
            echo "trace-exec not running"
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|status}"
        exit 1
        ;;
esac
