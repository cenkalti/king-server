#!/usr/bin/env bash
# Runs a real match: `king serve` plus 4 separate examplebot processes.
#
# Environment:
#   GAMES=100     number of games
#   SEED=42       match seed
#   KILL=1        kill one bot after KILL_AT games and restart it 10s later
#   KILL_AT=30
#   TUNNEL=1      route bots through a Cloudflare quick tunnel (needs cloudflared)
#   PARALLEL=1    games at once (passed to king serve if > 1)
#   PORT=28417
#   OUT=matches   results directory
set -euo pipefail

GAMES=${GAMES:-100}
SEED=${SEED:-42}
KILL=${KILL:-0}
KILL_AT=${KILL_AT:-30}
TUNNEL=${TUNNEL:-0}
PARALLEL=${PARALLEL:-1}
PORT=${PORT:-28417}
OUT=${OUT:-matches}

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
pids=()
cleanup() {
	for p in ${pids[@]+"${pids[@]}"}; do kill "$p" 2>/dev/null || true; done
	rm -rf "$work"
}
trap cleanup EXIT

(cd "$root" && go build -o "$work/king" ./cmd/king && go build -o "$work/examplebot" ./cmd/examplebot)

extra=()
if [ "$PARALLEL" -gt 1 ]; then extra+=(-parallel "$PARALLEL"); fi
"$work/king" serve -addr "127.0.0.1:$PORT" -games "$GAMES" -seed "$SEED" -out "$OUT" ${extra[@]+"${extra[@]}"} \
	>"$work/serve.out" 2>"$work/serve.err" &
serve=$!
pids+=("$serve")

for _ in $(seq 50); do
	grep -q "waiting for" "$work/serve.err" 2>/dev/null && break
	sleep 0.1
done
kill -0 "$serve" 2>/dev/null || { echo "king serve failed to start:"; cat "$work/serve.err"; exit 1; }

url="ws://127.0.0.1:$PORT/ws"
if [ "$TUNNEL" = 1 ]; then
	cloudflared tunnel --url "http://127.0.0.1:$PORT" >"$work/tunnel.log" 2>&1 &
	pids+=("$!")
	host=""
	for _ in $(seq 300); do
		host=$(grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' "$work/tunnel.log" | head -1 || true)
		[ -n "$host" ] && break
		sleep 0.1
	done
	[ -n "$host" ] || { echo "tunnel did not come up"; cat "$work/tunnel.log"; exit 1; }
	url="wss://${host#https://}/ws"
	# Wait until the tunnel actually routes (DNS for a fresh host can lag).
	for _ in $(seq 60); do
		curl -fsS "$host/" >/dev/null 2>&1 && break
		sleep 1
	done
	echo "tunnel: $url"
fi

names=(alice bob cenk deniz)
strategies=(random low random low)
start_bot() {
	local i=$1
	"$work/examplebot" -url "$url" -name "${names[$i]}" -strategy "${strategies[$i]}" -seed $((i + 1)) \
		>>"$work/bot-${names[$i]}.log" 2>&1 &
	botpid[$i]=$!
	pids+=("$!")
}
declare -a botpid
for i in 0 1 2 3; do start_bot "$i"; sleep 0.2; done

start=$(date +%s)
if [ "$KILL" = 1 ]; then
	until grep -q "game $KILL_AT/$GAMES done" "$work/serve.err"; do
		kill -0 "$serve" 2>/dev/null || break
		sleep 0.2
	done
	echo "killing ${names[0]} after game $KILL_AT"
	kill -9 "${botpid[0]}"
	sleep 10
	echo "restarting ${names[0]}"
	start_bot 0
fi

status=0
wait "$serve" || status=$?
elapsed=$(($(date +%s) - start))

cat "$work/serve.out"
echo
grep -E "PAUSED|resumed|rejected" "$work/serve.err" || true
echo "serve exit status: $status, elapsed: ${elapsed}s"

dir=$(sed -n 's/^results in \([^ ]*\).*/\1/p' "$work/serve.out")
[ -n "$dir" ] || { echo "no results dir"; cat "$work/serve.err"; exit 1; }
for f in results.json standings.txt log.jsonl; do
	[ -s "$dir/$f" ] || { echo "missing $dir/$f"; exit 1; }
done
grep -q '"complete": true' "$dir/results.json" || { echo "match incomplete"; exit 1; }
if [ "$KILL" = 1 ]; then
	grep -q '"type":"pause"' "$dir/log.jsonl" || { echo "no pause recorded"; exit 1; }
fi
echo "OK: $dir"
exit "$status"
