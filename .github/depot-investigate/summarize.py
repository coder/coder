import collections,json,pathlib,re,sys
p=pathlib.Path(sys.argv[1]);files=list(p.glob('protocol-*.json'));a=[json.loads(f.read_text()) for f in files]
def dist(xs):
 xs=sorted(xs)
 return {'n':len(xs),**({k:round(xs[min(len(xs)-1,int((len(xs)-1)*q))]*1000,4) for k,q in [('p50_ms',.5),('p95_ms',.95),('p99_ms',.99)]} if xs else {})}
def duration(s):
 return sum(float(n)*{'h':3600,'m':60,'s':1,'ms':.001,'us':.000001,'µs':.000001,'ns':.000000001}[unit] for n,unit in re.findall(r'([0-9.]+)(ms|us|µs|ns|h|m|s)',s))
reqs=[r for f in a for r in f['requests']];rem=[r for f in a for r in f['remote']]
res={'commands':dict(collections.Counter(x['command'] for x in reqs)),'get_hits':dist([r['seconds'] for r in reqs if r['command']=='get' and not r['miss'] and not r['error']]),'get_misses':dist([r['seconds'] for r in reqs if r['command']=='get' and r['miss']]),'get_errors':sum(r['error'] for r in reqs if r['command']=='get'),'remote_gets':dist([duration(r['duration_text']) for r in rem if r['method']=='GET']),'remote_get_bytes':sum(r['bytes'] for r in rem if r['method']=='GET'),'remote_puts':dist([duration(r['duration_text']) for r in rem if r['method']=='PUT']),'peak_active_gets':max((f['peak_active_gets'] for f in a),default=0)}
(p/'summary.json').write_text(json.dumps(res,indent=2)+'\n');print(json.dumps(res,indent=2))
