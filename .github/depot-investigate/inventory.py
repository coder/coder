import collections,json,os,pathlib,re,shlex,shutil,subprocess,time,urllib.parse
out=pathlib.Path(os.environ['RUNNER_TEMP'])/'depot-investigation';out.mkdir(exist_ok=True)
def run(cmd,timeout=25):
 try:
  r=subprocess.run(cmd,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=timeout)
  return {'returncode':r.returncode,'output':r.stdout[:20000]}
 except Exception as e:return {'error_type':type(e).__name__}
def safe_args(raw):
 try:args=shlex.split(raw)
 except ValueError:return ['unparseable']
 safe=[]
 for i,a in enumerate(args):
  if i==0:safe.append(a)
  elif a in ['gocache','--verbose','-v']:safe.append(a)
  elif a.startswith('-'):safe.append(a.split('=')[0]+('=<redacted>' if '=' in a else ''))
  else:safe.append('<argument>')
 return safe
result={'time':time.time(),'GOCACHEPROG':safe_args(os.environ.get('GOCACHEPROG','')),'paths':{k:os.environ.get(k) for k in ['GOCACHE','GOMODCACHE','GOPATH','GOTMPDIR','RUNNER_TEMP']},'cache_env_names':sorted(k for k in os.environ if re.search('DEPOT|GOCACHE|CACHE_HOST',k)),'tools':{x:shutil.which(x) for x in ['depot','go','strace','tcpdump','ss','curl','python3']},'processes':[]}
if os.environ.get('DEPOT_CACHE_HOST'):
 u=urllib.parse.urlsplit(os.environ['DEPOT_CACHE_HOST']);result['cache_origin']=u.scheme+'://'+u.netloc
for f in pathlib.Path('/proc').glob('[0-9]*/comm'):
 try:
  name=f.read_text().strip()
  if re.search('depot|cache|runner|proxy',name,re.I):
   base=f.parent;raw=(base/'cmdline').read_bytes().split(b'\0');args=[a.decode(errors='replace') for a in raw if a]
   result['processes'].append({'pid':int(base.name),'comm':name,'executable':str((base/'exe').resolve()),'args':safe_args(shlex.join(args))})
 except (OSError,PermissionError):pass
for label,cmd in [('depot_version',['depot','version']),('gocache_help',['depot','gocache','--help']),('go_version',['go','version']),('listeners',['sudo','ss','-lntp']),('unix_listeners',['sudo','ss','-lxnp']),('filesystems',['findmnt','--json']),('network',['ip','route']),('depot_go_build_info',['go','version','-m',shutil.which('depot') or '/usr/local/bin/depot'])]:
 result[label]=run(cmd)
for label,path in [('default_cache',pathlib.Path.home()/'.cache/go-build'),('GOCACHE',pathlib.Path(os.environ.get('GOCACHE') or '/nonexistent'))]:
 if path.exists():
  total=0;count=0;kinds=collections.Counter()
  for f in path.rglob('*'):
   if f.is_file():
    count+=1;total+=f.stat().st_size;kinds['depot' if f.name.startswith('depot-') else 'go']+=1
  result[label]={'path':str(path),'files':count,'bytes':total,'kinds':dict(kinds)}
(out/'inventory.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
