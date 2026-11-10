import json,os,pathlib,re,subprocess,urllib.parse
out=pathlib.Path(os.environ['RUNNER_TEMP'])/'depot-investigation'
def run(cmd,timeout=20):
 try:
  r=subprocess.run(cmd,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,timeout=timeout)
  return {'exit_code':r.returncode,'output':r.stdout[:30000]}
 except Exception as e:return {'error':type(e).__name__}
result={'addresses':run(['ip','-j','addr']),'route_to_cache':run(['ip','route','get',urllib.parse.urlsplit(os.environ['DEPOT_CACHE_HOST']).hostname]),'agents':[]}
for f in pathlib.Path('/proc').glob('[0-9]*/comm'):
 try:
  if not f.read_text().startswith('agentd'):continue
  pid=f.parent.name;exe='/proc/'+pid+'/exe'
  entry={'pid':int(pid),'executable':run(['sudo','readlink',exe]),'build_info':run(['sudo','go','version','-m',exe]),'fd_targets':[]}
  # These are paths only, never file contents or process environment.
  entry['fd_targets']=run(['sudo','ls','-l',str(f.parent/'fd')])
  entry['status']=run(['sudo','cat',str(f.parent/'status')])
  entry['cache_symbols']=run(['bash','-c','sudo strings '+exe+" | rg '^(github.com/depot/|github.com/.*/(ristretto|bigcache|fastcache|pebble|badger|groupcache|redis)|.*[./](goCache|gocache|cacheProxy|cacheproxy|prefetch)[A-Za-z0-9_/().]*)$' | head -300"],timeout=40)
  result['agents'].append(entry)
 except (OSError,PermissionError):pass
(out/'agent.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps({k:v for k,v in result.items() if k!='agents'},indent=2))
print('Agent inspection complete; process metadata saved without environment or credential values.')
