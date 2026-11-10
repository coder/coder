import collections,concurrent.futures,gzip,hashlib,http.client,io,json,os,pathlib,random,ssl,struct,time,urllib.parse,uuid
out=pathlib.Path(os.environ['RUNNER_TEMP'])/'depot-investigation'
records=[]
for f in (out/'cold-plugin').glob('protocol-*.json'):
 records+=json.loads(f.read_text())['requests']
by_id={r['action_id']:r for r in records if r['command']=='get' and not r['miss'] and not r['error'] and r['action_id']}
small=sorted([r for r in by_id.values() if 0<r['size']<=65536],key=lambda r:hashlib.sha256(r['action_id'].encode()).hexdigest())[:96]
medium=sorted([r for r in by_id.values() if 65536<r['size']<=1048576],key=lambda r:hashlib.sha256(r['action_id'].encode()).hexdigest())[:24]
large=sorted([r for r in by_id.values() if 1048576<r['size']<=33554432],key=lambda r:hashlib.sha256(r['action_id'].encode()).hexdigest())[:8]
samples=small+medium+large
# The same job credential is used only with Depot's configured service and its documented public cache service.
token=os.environ.get('DEPOT_CACHE_TOKEN') or os.environ.get('DEPOT_JIT_TOKEN') or os.environ.get('DEPOT_TOKEN')
if not token:raise RuntimeError('No documented Depot cache credential available')
org=os.environ.get('DEPOT_ORG_ID','')
allowed_headers={'content-type','content-length','content-encoding','server','via','age','x-cache','x-cache-hits','x-served-by','x-amz-cf-pop','x-depot-cache','x-depot-cache-status','server-timing','accept-ranges','cache-control','transfer-encoding'}
origins={'runner-agent':os.environ['DEPOT_CACHE_HOST'],'public-cache':'https://cache.depot.dev'}
results=[]
def conn_for(origin):
 u=urllib.parse.urlsplit(origin)
 return (http.client.HTTPSConnection(u.hostname,u.port or 443,timeout=30) if u.scheme=='https' else http.client.HTTPConnection(u.hostname,u.port or 80,timeout=30))
def one(conn,origin,rec,encoding):
 path='/gocache/v1/'+rec['action_id'];headers={'Authorization':'Bearer '+token,'User-Agent':'gocacheprog','Accept-Encoding':encoding}
 if org:headers['X-Depot-Org']=org
 start=time.perf_counter();conn.request('GET',path,headers=headers);res=conn.getresponse();ttfb=time.perf_counter()-start
 response_headers={k.lower():v for k,v in res.getheaders() if k.lower() in allowed_headers}
 payload=res.read();elapsed=time.perf_counter()-start
 decoded=gzip.decompress(payload) if res.getheader('Content-Encoding')=='gzip' else payload
 result={'origin':origin,'action_id':rec['action_id'],'expected_size':rec['size'],'status':res.status,'ttfb_seconds':ttfb,'seconds':elapsed,'wire_bytes':len(payload),'decoded_bytes':len(decoded),'headers':response_headers}
 if res.status in [301,302,303,307,308]:
  u=urllib.parse.urlsplit(res.getheader('Location',''));result['redirect_origin']=u.scheme+'://'+u.netloc
 elif res.status==200:
  if decoded:
   oid_len=decoded[0];declared=struct.unpack('<I',decoded[1+oid_len:5+oid_len])[0];result['envelope']={'output_id_length':oid_len,'declared_output_bytes':declared,'actual_output_bytes':len(decoded)-5-oid_len}
  else:result['envelope']={'declared_output_bytes':0,'actual_output_bytes':0}
 return result
for label,origin in origins.items():
 c=conn_for(origin)
 try:
  for r in samples:
   try:results.append(one(c,label,r,'gzip'))
   except Exception as e:results.append({'origin':label,'error_type':type(e).__name__});c.close();c=conn_for(origin)
  # Repeated reads bypass the CLI's disk cache and exercise the service directly.
  for r in small[:8]:
   for i in range(4):
    v=one(c,label,r,'identity');v['repeat']=i;v['accept_encoding']='identity';results.append(v)
  for i in range(8):
   rec={'action_id':hashlib.sha256(('depot-investigation-miss-'+uuid.uuid4().hex).encode()).hexdigest(),'size':0}
   v=one(c,label,rec,'gzip');v['expected_miss']=True;results.append(v)
 finally:c.close()
(out/'http-probes.json').write_text(json.dumps(results,indent=2)+'\n')
def dist(xs):
 xs=sorted(xs)
 return {'n':len(xs),**({k:round(xs[int((len(xs)-1)*q)]*1000,4) for k,q in [('p50_ms',.5),('p95_ms',.95)]} if xs else {})}
summary={}
for label in origins:
 rs=[r for r in results if r['origin']==label];hits=[r for r in rs if r.get('status')==200 and 'repeat' not in r]
 summary[label]={'status_counts':dict(collections.Counter(str(r.get('status',r.get('error_type'))) for r in rs)),'hit_ttfb':dist([r['ttfb_seconds'] for r in hits]),'hit_total':dist([r['seconds'] for r in hits]),'small_hit_ttfb':dist([r['ttfb_seconds'] for r in hits if r['expected_size']<=65536]),'wire_bytes':sum(r['wire_bytes'] for r in hits),'decoded_bytes':sum(r['decoded_bytes'] for r in hits),'encodings':dict(collections.Counter(r['headers'].get('content-encoding','identity') for r in hits)),'repeat_ttfb':dist([r['ttfb_seconds'] for r in rs if 'repeat' in r and r.get('status')==200]),'miss_ttfb':dist([r['ttfb_seconds'] for r in rs if r.get('expected_miss')])}
(out/'http-summary.json').write_text(json.dumps(summary,indent=2)+'\n');print(json.dumps(summary,indent=2))
