# Run under sudo. Read only process metadata, executable symbols, and cache directory statistics.
import collections,json,os,pathlib,re,subprocess,sys
out=pathlib.Path(sys.argv[1]);label=sys.argv[2];result={'agents':[],'cache_dirs':[]}
for f in pathlib.Path('/proc').glob('[0-9]*/comm'):
 try:
  if not f.read_text().startswith('agentd'):continue
  exe=f.parent/'exe';data=exe.read_bytes()
  names=set(re.findall(rb'(?:depot\.dev|github\.com/depot)/[A-Za-z0-9_./()*-]{1,250}',data))
  symbols=sorted(n.decode() for n in names if re.search(rb'cache|bundle|segment|prefetch',n,re.I))
  entry={'pid':int(f.parent.name),'executable':str(exe.resolve()),'cache_symbols':symbols[:1500],'cache_symbol_count':len(symbols),'fd_targets':[]}
  for link in (f.parent/'fd').iterdir():
   try:entry['fd_targets'].append(os.readlink(link))
   except OSError:pass
  entry['memory_status']=[l for l in (f.parent/'status').read_text().splitlines() if l.startswith(('VmRSS:','VmSize:','RssAnon:','RssFile:','Threads:'))]
  result['agents'].append(entry)
 except OSError:pass
for root in ['/tmp','/var/cache','/var/lib/depot','/mnt','/run/depot']:
 path=pathlib.Path(root)
 if not path.exists():continue
 for base,dirs,files in os.walk(path):
  depth=len(pathlib.Path(base).relative_to(path).parts)
  if depth>=2:dirs[:]=[]
  if re.search(r'cache|bundle|segment|depot',base,re.I):
   counts=collections.Counter();total=0
   for name in files:
    try:
     s=(pathlib.Path(base)/name).stat();total+=s.st_size;counts['files']+=1
    except OSError:pass
   result['cache_dirs'].append({'path':base,'direct_bytes':total,'file_count':counts['files'],'child_dirs':len(dirs),'sample_names':files[:3]})
(out/('agent-details-'+label+'.json')).write_text(json.dumps(result,indent=2)+'\n')
print('Saved agent symbols and directory statistics:',label)
