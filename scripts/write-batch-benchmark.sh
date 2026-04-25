#!/usr/bin/env bash
set -euo pipefail

BASE=${BASE:-http://127.0.0.1:18811}
USER=${USER_NAME:-admin}
PASS=${USER_PASS:-admin123}
N=${N:-50}

LOGIN=$(curl -fsS -H 'content-type: application/json' -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" "$BASE/auth/login")
TOK=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["token"])' <<< "$LOGIN")
AUTH="Authorization: Bearer $TOK"

python3 - <<PY
import subprocess,json,time,statistics
BASE='${BASE}'
AUTH='${AUTH}'
N=int('${N}')

def api(method,path,body=None):
    cmd=['curl','-sS','-H',AUTH,'-H','content-type: application/json','-X',method,BASE+path]
    if body is not None:
        cmd+=['-d',json.dumps(body)]
    p=subprocess.run(cmd,capture_output=True,text=True)
    if p.returncode!=0:
        raise RuntimeError(p.stderr)
    return json.loads(p.stdout)

# single inserts
single=[]
single_ids=[]
for i in range(N):
    t=time.perf_counter()
    j=api('POST','/api/inbounds/add',{'remark':f'wb-single-{i}','protocol':'socks','auth':'noauth'})
    single.append((time.perf_counter()-t)*1000)
    _id=((j.get('obj') or {}).get('id'))
    if _id: single_ids.append(_id)

# cleanup
for _id in single_ids:
    api('DELETE',f'/api/inbounds/{_id}')

# batch inserts
items=[{'remark':f'wb-batch-{i}','protocol':'socks','auth':'noauth'} for i in range(N)]
t=time.perf_counter()
j=api('POST','/api/inbounds/add-batch',{'items':items})
batch_ms=(time.perf_counter()-t)*1000
batch_ids=[x.get('id') for x in (j.get('obj') or []) if x.get('id')]
for _id in batch_ids:
    api('DELETE',f'/api/inbounds/{_id}')

out={
  'n':N,
  'single_p50_ms':round(statistics.median(single),2),
  'single_p95_ms':round(sorted(single)[int(len(single)*0.95)-1],2),
  'single_total_ms':round(sum(single),2),
  'batch_total_ms':round(batch_ms,2),
  'speedup_x': round((sum(single)/batch_ms),2) if batch_ms>0 else None
}
print(json.dumps(out,ensure_ascii=False,indent=2))
PY
