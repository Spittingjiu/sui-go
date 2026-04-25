#!/usr/bin/env bash
set -euo pipefail

BASE=${BASE:-http://127.0.0.1:18811}
USER=${USER_NAME:-admin}
PASS=${USER_PASS:-admin123}

LOGIN=$(curl -fsS -H 'content-type: application/json' -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" "$BASE/auth/login")
TOK=$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["token"])' <<< "$LOGIN")
AUTH="Authorization: Bearer $TOK"

python3 - <<PY
import subprocess, json
BASE='${BASE}'
AUTH='${AUTH}'

def req(method,path,body=None):
    cmd=['curl','-sS','-H',AUTH,'-H','content-type: application/json','-X',method,BASE+path]
    if body is not None:
        cmd+=['-d',json.dumps(body)]
    p=subprocess.run(cmd,capture_output=True,text=True)
    return p.returncode, p.stdout

# 1) unknown override key should be rejected
rc,out=req('POST','/api/inbounds/add',{'remark':'fi-1','protocol':'vless','settingsOverride':{'clients':[],'bad':1}})
obj=json.loads(out)
print('reject_bad_override', (obj.get('success') is False and 'unknown keys' in (obj.get('msg') or '').lower()))

# 2) invalid shortId rejected
rc,out=req('POST','/api/inbounds/add',{'remark':'fi-2','protocol':'vless','security':'reality','shortId':'zzzzzzzzzzzzzzzzzz'})
obj=json.loads(out)
print('reject_bad_shortid', (obj.get('success') is False and 'shortid' in (obj.get('msg') or '').lower()))

# 3) add-reality-quick path should still succeed
rc,out=req('POST','/api/inbounds/add-reality-quick',{'remark':'fi-reality'})
obj=json.loads(out)
print('reality_quick_ok', obj.get('success') is True)
if obj.get('success'):
    _id=obj['obj']['id']
    req('DELETE',f'/api/inbounds/{_id}')
PY
