#!/bin/bash

# Run from cron. Restarts the server when it is not running or not responding, and alerts when the
# server is up but cannot reach MongoDB (see "Health check" in DEPLOYMENT.md).
#
# IMPORTANT: need to change the permissions of the log.txt if it's recreated
# chmod 666 "$output_file"
#
# Optional env vars:
#   HEALTH_ALERT_WEBHOOK_URL - Slack-compatible webhook that is posted to when MongoDB is unreachable
#   HEALTH_URL               - defaults to http://localhost:3002/api/health

# Define the output file
output_file="/schej.it/deploy_scripts/reboot_log.txt"
# Number of consecutive checks in which the server did not respond
failure_state_file="/schej.it/deploy_scripts/health_failures.txt"
health_url="${HEALTH_URL:-http://localhost:3002/api/health}"
# Restart only after this many consecutive checks without a response, so one slow check doesn't restart it
max_failures=2

# Get current timestamp
timestamp=$(date "+%Y-%m-%d %H:%M:%S")

restart_server() {
    cd /schej.it/server
    screen -dmS schej.it-server sudo ./server -release=true
}

cd /schej.it/deploy_scripts

# Check if the screen session is running
if ! screen -ls | grep -q "schej.it-server"; then
    echo "[$timestamp] Screen 'schej.it-server' is not running! Rebooting..." >> "$output_file"
    echo 0 > "$failure_state_file"
    restart_server
    exit 0
fi

# The screen session exists, check that the server responds. curl prints 000 when there is no response
status=$(curl -s -o /dev/null -w "%{http_code}" --max-time 10 "$health_url")

case "$status" in
    200)
        echo "[$timestamp] Screen 'schej.it-server' is running and healthy." >> "$output_file"
        echo 0 > "$failure_state_file"
        ;;
    503)
        # Restarting won't fix mongo, and the server reconnects on its own once mongo is back
        echo "[$timestamp] Server up, MongoDB unreachable (/api/health returned 503). Not restarting." >> "$output_file"
        echo 0 > "$failure_state_file"
        if [ -n "$HEALTH_ALERT_WEBHOOK_URL" ]; then
            curl -s --max-time 10 -X POST -H "Content-Type: application/json" \
                -d "{\"text\":\"[$timestamp] Timeful: server is up but MongoDB is unreachable (/api/health returned 503)\"}" \
                "$HEALTH_ALERT_WEBHOOK_URL" > /dev/null
        fi
        ;;
    *)
        failures=$(cat "$failure_state_file" 2>/dev/null)
        failures=$(( ${failures:-0} + 1 ))
        if [ "$failures" -ge "$max_failures" ]; then
            echo "[$timestamp] Server not responding (status $status) for $failures consecutive checks! Rebooting..." >> "$output_file"
            echo 0 > "$failure_state_file"
            screen -S schej.it-server -X quit
            sleep 2
            restart_server
        else
            echo "[$timestamp] Server not responding (status $status), check $failures of $max_failures before restart." >> "$output_file"
            echo "$failures" > "$failure_state_file"
        fi
        ;;
esac
