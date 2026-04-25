#!/usr/bin/env bash
set -euo pipefail

BASE=${BASE:-http://127.0.0.1:18811}
USER=${USER_NAME:-admin}
PASS=${USER_PASS:-admin123}

LOGIN=$(curl -fsS -H 'content-type: application/json' -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" "$BASE/auth/login")
TOK=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["token"])' <<< "$LOGIN")
AUTH="Authorization: Bearer $TOK"

python3 - <<PY
import subprocess,time,statistics,json
AUTH='${AUTH}'
BASE='${BASE}'

def hit(path):
    t0=time.perf_counter()
    out=subprocess.check_output(['curl','-fsS','-H',AUTH,BASE+path], text=True)
    ms=(time.perf_counter()-t0)*1000
    return ms, out

# list full
arr=[]
for _ in range(20):
    ms,_=hit('/api/inbounds')
    arr.append(ms)
print('inbounds_full_ms',{'p50':round(statistics.median(arr),2),'p95':round(sorted(arr)[int(len(arr)*0.95)-1],2),'max':round(max(arr),2)})

# list lite
arr=[]
for _ in range(20):
    ms,out=hit('/api/inbounds?lite=1&limit=100')
    arr.append(ms)
print('inbounds_lite_ms',{'p50':round(statistics.median(arr),2),'p95':round(sorted(arr)[int(len(arr)*0.95)-1],2),'max':round(max(arr),2)})

# apply skip check
p1=subprocess.check_output(['curl','-fsS','-H',AUTH,'-X','POST',BASE+'/api/xray/apply'], text=True)
p2=subprocess.check_output(['curl','-fsS','-H',AUTH,'-X','POST',BASE+'/api/xray/apply'], text=True)
print('apply_1', json.loads(p1).get('msg','ok'), 'applied=', json.loads(p1).get('applied'))
print('apply_2', json.loads(p2).get('msg','ok'), 'applied=', json.loads(p2).get('applied'), 'skipped=', json.loads(p2).get('skipped'))
PY
