import collections,hashlib,http.client,json,os,pathlib,struct,time,urllib.parse,uuid
out=pathlib.Path(os.environ['RUNNER_TEMP'])/'depot-investigation';samples=json.loads((pathlib.Path(os.environ['DEPOT_PROBE_SCRIPTS'])/'replay-samples.json').read_text())
u=urllib.parse.urlsplit(os.environ['DEPOT_CACHE_HOST']);c=http.client.HTTPConnection(u.hostname,u.port,timeout=30)
token=os.environ.get('DEPOT_CACHE_TOKEN') or os.environ.get('DEPOT_JIT_TOKEN') or os.environ.get('DEPOT_TOKEN')
if not token:raise RuntimeError('Missing expected cache token')
headers={'Authorization':'Bearer '+token,'User-Agent':'gocacheprog','Accept-Encoding':'identity'}
if os.environ.get('DEPOT_ORG_ID'):headers['X-Depot-Org']=os.environ['DEPOT_ORG_ID']
records=[]
for index,sample in enumerate(samples):
 for repeat in [0,1]:
  start=time.perf_counter();wall=time.time();c.request('GET','/gocache/v1/'+sample['action_id'],headers=headers);r=c.getresponse();ttfb=time.perf_counter()-start;body=r.read()
  record={'index':index,'repeat':repeat,'action_id':sample['action_id'],'start_unix':wall,'ttfb_seconds':ttfb,'seconds':time.perf_counter()-start,'status':r.status,'bytes':len(body),'expected_output_size':sample['size']}
  if r.status==200 and body:
   n=body[0];record['declared_output_size']=struct.unpack('<I',body[1+n:5+n])[0];record['actual_output_size']=len(body)-5-n
  records.append(record)
 if index%32==0:print('Replayed entries',index,flush=True)
c.close()
# Write before any additional diagnostics so partial results survive later errors.
(out/'fresh-agent-replay.json').write_text(json.dumps(records,indent=2)+'\n')
def dist(xs):
 xs=sorted(xs)
 return {'n':len(xs),**({k:round(xs[int((len(xs)-1)*q)]*1000,4) for k,q in [('p50_ms',.5),('p95_ms',.95),('p99_ms',.99)]} if xs else {})}
summary={'status_counts':dict(collections.Counter(r['status'] for r in records)),'first_reads':dist([r['ttfb_seconds'] for r in records if r['repeat']==0 and r['status']==200]),'immediate_repeats':dist([r['ttfb_seconds'] for r in records if r['repeat']==1 and r['status']==200]),'first_reads_under_1ms':sum(r['repeat']==0 and r['status']==200 and r['ttfb_seconds']<.001 for r in records),'first_reads_over_5ms':sum(r['repeat']==0 and r['status']==200 and r['ttfb_seconds']>.005 for r in records),'returned_body_bytes':sum(r['bytes'] for r in records)}
(out/'fresh-agent-summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary,indent=2))
