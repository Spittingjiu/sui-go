#!/usr/bin/env bash
set -euo pipefail

BASE=${BASE:-http://127.0.0.1:18811}
USER=${USER_NAME:-admin}
PASS=${USER_PASS:-admin123}
TARGETS=${TARGETS:-100,500,1000}

LOGIN=$(curl -fsS -H 'content-type: application/json' -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" "$BASE/auth/login")
TOK=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["token"])' <<< "$LOGIN")
AUTH="Authorization: Bearer $TOK"

python3 - <<PY
import subprocess, json, time, statistics
BASE='${BASE}'
AUTH='${AUTH}'
TARGETS='${TARGETS}'.split(',')

def api(method,path,body=None):
    cmd=['curl','-sS','-H',AUTH,'-H','content-type: application/json','-X',method,BASE+path]
    if body is not None:
        cmd += ['-d', json.dumps(body)]
    p=subprocess.run(cmd, capture_output=True, text=True)
    if p.returncode!=0:
        raise RuntimeError(p.stderr)
    return json.loads(p.stdout)

rows=api('GET','/api/inbounds').get('obj') or []
cur=len(rows)

# fill to max target with lightweight socks entries
max_target=max(int(x) for x in TARGETS if x.strip())
for i in range(cur, max_target):
    api('POST','/api/inbounds/add',{'remark':f'scale-{i}','protocol':'socks','auth':'noauth'})

report=[]
for t in [int(x) for x in TARGETS if x.strip()]:
    # sample full + lite latency
    full=[]
    lite=[]
    for _ in range(10):
        s=time.perf_counter(); api('GET','/api/inbounds'); full.append((time.perf_counter()-s)*1000)
        s=time.perf_counter(); api('GET',f'/api/inbounds?lite=1&limit={t}'); lite.append((time.perf_counter()-s)*1000)
    report.append({
      'size': t,
      'full_p50_ms': round(statistics.median(full),2),
      'full_p95_ms': round(sorted(full)[int(len(full)*0.95)-1],2),
      'lite_p50_ms': round(statistics.median(lite),2),
      'lite_p95_ms': round(sorted(lite)[int(len(lite)*0.95)-1],2),
    })

out={'suite':'scale-baseline','report':report}
print(json.dumps(out,ensure_ascii=False,indent=2))
open('/opt/sui-go/docs/perf-scale-report-2026-04-25.json','w',encoding='utf-8').write(json.dumps(out,ensure_ascii=False,indent=2))
PY
