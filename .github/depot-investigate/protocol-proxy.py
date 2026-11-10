#!/usr/bin/env python3
import base64,collections,json,os,pathlib,re,shlex,subprocess,sys,threading,time
out=pathlib.Path(os.environ['DEPOT_PROBE_PHASE_DIR']);out.mkdir(exist_ok=True)
cmd=shlex.split(os.environ['DEPOT_ORIGINAL_GOCACHEPROG'])+['--verbose']
p=subprocess.Popen(cmd,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
started=time.perf_counter();pending={};events=[];remote=[];misc=collections.Counter();lock=threading.Lock();peak=0
# Only protocol metadata and timing are retained; no PUT bodies or credentials.
def forward_input():
 global peak
 try:
  for line in sys.stdin.buffer:
   if line.lstrip().startswith(b'{'):
    req=json.loads(line)
    with lock:
     pending[req['ID']]={'command':req['Command'],'action_id':base64.b64decode(req.get('ActionID','')).hex(),'start':time.perf_counter()}
     peak=max(peak,sum(x['command']=='get' for x in pending.values()))
   p.stdin.write(line);p.stdin.flush()
 except BrokenPipeError:pass
 finally:
  try:p.stdin.close()
  except OSError:pass
def stderr():
 for raw in p.stderr:
  line=raw.decode(errors='replace')
  m=re.search(r'\b(GET|PUT) /gocache/v1/([a-f0-9]+): (\d+) bytes in ([0-9.hmsuµn]+)',line)
  if m:
   remote.append({'method':m[1],'action_id':m[2],'bytes':int(m[3]),'duration_text':m[4]})
  elif 'disk miss:' in line:misc['disk_miss']+=1
  elif 'cacher: closing;' in line:misc['close_summary']+=1
  elif 'using depot gocache' not in line:misc['other_stderr_lines']+=1
threads=[threading.Thread(target=forward_input,daemon=True),threading.Thread(target=stderr,daemon=True)]
for t in threads:t.start()
for line in p.stdout:
 res=json.loads(line)
 if 'KnownCommands' not in res:
  with lock:req=pending.pop(res['ID'],None)
  if req:
   req['seconds']=time.perf_counter()-req.pop('start');req.update(miss=bool(res.get('Miss')),error=bool(res.get('Err')),size=res.get('Size',0))
   events.append(req)
 sys.stdout.buffer.write(line);sys.stdout.buffer.flush()
code=p.wait()
for t in threads:t.join(timeout=3)
(out/('protocol-'+str(os.getpid())+'.json')).write_text(json.dumps({'seconds':time.perf_counter()-started,'exit_code':code,'peak_active_gets':peak,'requests':events,'remote':remote,'stderr_counts':dict(misc)},separators=(',',':')))
sys.exit(code)
