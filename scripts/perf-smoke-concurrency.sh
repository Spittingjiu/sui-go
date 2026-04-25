#!/usr/bin/env bash
set -euo pipefail

BASE=${BASE:-http://127.0.0.1:18811}
USER=${USER_NAME:-admin}
PASS=${USER_PASS:-admin123}
N=${N:-20}

LOGIN=$(curl -fsS -H 'content-type: application/json' -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" "$BASE/auth/login")
TOK=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["token"])' <<< "$LOGIN")
AUTH="Authorization: Bearer $TOK"

python3 - <<PY
import subprocess, threading, time, json, statistics
BASE='${BASE}'
AUTH='${AUTH}'
N=int('${N}')

full=[]
lite=[]
apply=[]


def hit(path, arr):
    t0=time.perf_counter()
    p=subprocess.run(['curl','-sS','-H',AUTH,BASE+path], capture_output=True, text=True)
    dt=(time.perf_counter()-t0)*1000
    arr.append((dt,p.returncode,p.stdout))

threads=[]
for _ in range(N):
    threads.append(threading.Thread(target=hit,args=('/api/inbounds',full)))
    threads.append(threading.Thread(target=hit,args=('/api/inbounds?lite=1&limit=100',lite)))
for t in threads: t.start()
for t in threads: t.join()

threads=[]
for _ in range(max(3,N//5)):
    threads.append(threading.Thread(target=hit,args=('/api/xray/apply',apply)))
for t in threads: t.start()
for t in threads: t.join()

def stat(name, arr):
    ms=[x[0] for x in arr if x[1]==0]
    if not ms:
        return {name:{'ok':0,'total':len(arr)}}
    ms2=sorted(ms)
    return {name:{'ok':len(ms),'total':len(arr),'p50':round(statistics.median(ms2),2),'p95':round(ms2[int(len(ms2)*0.95)-1],2),'max':round(max(ms2),2)}}

r={}
r.update(stat('inbounds_full',full))
r.update(stat('inbounds_lite',lite))
r.update(stat('xray_apply',apply))

skip=0
for _,rc,out in apply:
    if rc==0:
        try:
            j=json.loads(out)
            if j.get('skipped'):
                skip+=1
        except Exception:
            pass
r['xray_apply']['skipped']=skip
print(json.dumps(r,ensure_ascii=False,indent=2))
PY
