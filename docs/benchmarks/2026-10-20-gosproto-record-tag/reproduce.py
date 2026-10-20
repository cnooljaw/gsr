"""整数记录每字段tag局部复用实验；固定原19+18项、独立进程、相同采样函数，所有构建先于计时。"""
import argparse, importlib.util, json, os, subprocess, time
from pathlib import Path
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--library',type=Path,required=True)
parser.add_argument('--output',type=Path,required=True)
args=parser.parse_args()
here=Path(__file__).resolve().parent
root=here.parents[2]
tool=root/'examples/websocket-gate/benchmarks/tools/go-pairs/run.py'
spec=importlib.util.spec_from_file_location('pairs',tool);pairs=importlib.util.module_from_spec(spec);spec.loader.exec_module(pairs)
plan=json.loads((here/'plan.json').read_text());out=args.output.resolve();out.mkdir(exist_ok=False)
previous=here.parent/'2026-10-18-gosproto-post-layout-profile'
fixture_env={'GSR_GROWTH_FIXTURE_PLAN':str(previous/'fixture-plan.json'),'GSR_GROWTH_CASES':','.join(plan['growthCases'])}
telemetry=here.parent/'2026-10-19-gosproto-isolated-cases/telemetry-probe.go.txt'
bound={str(p):pairs.digest(p) for p in [here/'reproduce.py',here/'candidate.patch',here/'plan.json',tool,tool.with_name('sampler_test.go.txt'),telemetry,here/'growth-adapter.go.txt',*[previous/n for n in ['rpc-probe.go.txt','layout-fixtures.go.txt','growth-probe.go.txt','fixture-plan.json']]]}
commands=[]
for key,expected in [('GOGC','100'),('GOMEMLIMIT','off')]:
    assert os.environ.get(key,'') in {'',expected},key

def run(command,cwd,env=None,timeout=60):
    start=time.monotonic()
    r=subprocess.run(command,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout)
    commands.append(dict(args=command,cwd=str(cwd),env=env or {},elapsedSeconds=time.monotonic()-start,exitCode=r.returncode,output=r.stdout+r.stderr))
    (out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
    if r.returncode:raise RuntimeError(r.stdout+r.stderr)
    return r.stdout+r.stderr

def replace(s,a,b):
    assert s.count(a)==1,a
    return s.replace(a,b,1)

def check(d,m):
    for p,h in bound.items():assert pairs.digest(Path(p))==h,p
    for sub,key in [('before','beforeGoSource'),('candidate','candidateGoSource'),('harness','harnessGoSource')]:assert pairs.sources(d/sub)==m[key],sub
    for p,h in m['generatedGoInputs'].items():assert pairs.digest(root/p)==h,p
    for p,h in m['fixtures'].items():assert pairs.digest(d/'harness'/p)==h,p
    assert pairs.digest(d/'pairs.test')==m['binarySha256']

# 四种模式均先归档、构建及检查，不在计时中运行编译器。
for mode in plan['modes']:
    d=out/mode;kind=mode if mode in ['self','clone'] else 'compare'
    cmd=['python3','-B',str(tool),'--library',str(args.library.resolve()),'--baseline',plan['baselineCommit'],'--output',str(d),'--prepare-only','--mode',kind]
    if kind=='compare':cmd+=['--candidate-patch',str(here/'candidate.patch')]
    run(cmd,root)
    h=d/'harness';p=h/'sampler_test.go';s=p.read_text()
    s=replace(s,'func pairOperations(', 'func originalPairOperations(')
    s=replace(s,'type pairResult struct {','type pairResult struct {\n\tTelemetry []isolatedSnapshot `json:"telemetry"`')
    s=replace(s,'\t\truntime.GC()\n\t\t// 每项','\t\tencodedSink, representativeSink, decodedSink = nil, nil, ""\n\t\truntime.GC()\n\t\tresult.Telemetry = make([]isolatedSnapshot, 0, 4)\n\t\tresult.Telemetry = append(result.Telemetry, isolatedMetrics("after_gc_before_warmup"))\n\t\t// 每项')
    s=replace(s,'\t\tresult.CallsPerBlock = max(leftCalls, rightCalls)','\t\tresult.Telemetry = append(result.Telemetry, isolatedMetrics("after_calibration"))\n\t\tresult.CallsPerBlock = max(leftCalls, rightCalls)')
    s=replace(s,'\t\tfor repeat := range 3 {','\t\tresult.Telemetry = append(result.Telemetry, isolatedMetrics("after_timing"))\n\t\tfor repeat := range 3 {')
    s=replace(s,'\t\tresults = append(results, result)','\t\tresult.Telemetry = append(result.Telemetry, isolatedMetrics("after_allocation"))\n\t\tresults = append(results, result)')
    p.write_text(s);(h/'telemetry_probe_test.go').write_bytes(telemetry.read_bytes())
    (h/'growth_adapter_test.go').write_bytes((here/'growth-adapter.go.txt').read_bytes())
    (h/'rpc_probe_test.go').write_text((previous/'rpc-probe.go.txt').read_text().replace('func pairOperations(', 'func rpcPairOperations(',1))
    for name,target in [('layout-fixtures.go.txt','layout_fixtures_test.go'),('growth-probe.go.txt','growth_probe_test.go')]:
        (h/target).write_bytes((previous/name).read_bytes())
    run(['gofmt','-w',*[f.name for f in h.glob('*.go')]],h)
    run(['go','test','-mod=mod','-run','^(TestPairStatistics|TestPairSelection|TestPairGCCounter|TestRepresentativeFixtures|TestIsolatedMetrics|TestGrowthDifferential)$','.', '-timeout=1m'],h,fixture_env)
    run(['go','vet','-mod=mod','.'],h)
    run(['go','test','-mod=mod','-c','-o',str(d/'pairs.test'),'.'],h)
    m=json.loads((d/'inputs.json').read_text());m.update(harnessGoSource=pairs.sources(h),binarySha256=pairs.digest(d/'pairs.test'),selectedCases=plan['cases'],driverInputs=bound,diagnosticPlan=plan)
    (d/'inputs.json').write_text(json.dumps(m,indent=2)+'\n');check(d,m)
    run([str(d/'pairs.test'),'-test.run=^TestIsolatedCaseList$','-test.timeout=20s'],h,{**fixture_env,'GOMAXPROCS':'1','GSR_ISOLATED_LIST':str(d/'case-list.json')})
    assert json.loads((d/'case-list.json').read_text())==plan['cases']
    print(mode,'built',flush=True)
# 完整库切片验证只在比较副本进行；before 的测试模块名还原，不改二进制输入。
contracts='^(TestEncode|TestLimitsBeforeAllocation|TestIntegerRecordArrayBudgetsAndTags|TestCodecBudgetBoundaries|TestSchemaAndMinimumByteBudget|TestRpcFrame)'
for side in ['before','candidate']:
    d=out/'pass1'/side;mod=out/(side+'.mod')
    mod.write_text((d/'go.mod').read_text().replace('github.com/cnooljaw/gosproto-before','github.com/cnooljaw/gosproto'))
    run(['go','test','-modfile='+str(mod),'-run',contracts,'.','-timeout=1m'],d)
    compiler=run(['go','build','-gcflags=-m=2','.'],d);(out/(side+'-compiler.txt')).write_text(compiler)
    assembly=run(['go','tool','objdump','-s','github.com/cnooljaw/gosproto'+('-before' if side=='before' else '')+r'\.(encodedIntegerRecordSize|encodeIntegerRecordInto)',str(out/'pass1/pairs.test')],root)
    (out/(side+'-encode-assembly.txt')).write_text(assembly)
for mode in plan['modes']:
    d=out/mode;kind=mode if mode in ['self','clone'] else 'compare';m=json.loads((d/'inputs.json').read_text());aggregate=None;summary=[]
    for i,case in enumerate(plan['cases']):
        check(d,m);p=d/('case-%02d.json'%i)
        run([str(d/'pairs.test'),'-test.run=^TestPairSampling$','-test.v','-test.timeout=15s'],d/'harness',{**fixture_env,'GOMAXPROCS':'1','GSR_PAIR_MODE':kind,'GSR_PAIR_OUTPUT':str(p),'GSR_PAIR_BLOCK_TARGET':'10ms','GSR_PAIR_CASES':case},timeout=20)
        raw=json.loads(p.read_text());assert len(raw['results'])==1 and raw['results'][0]['name']==case
        assert raw['gogcPercent']==100 and raw['rounds']==6 and raw['quartetsPerRound']==4 and raw['targetBlockNs']==10000000
        result=raw['results'][0];assert [s['Stage'] for s in result['telemetry']]==plan['telemetry']
        assert all(s['GOGC']==100 and s['MemoryLimit']==2**63-1 for s in result['telemetry'])
        assert kind!='self' or result['sameFunction']
        if aggregate is None:aggregate={**raw,'results':[]}
        aggregate['results'].append(result);summary.extend(pairs.summarize(raw))
        (d/'raw.json').write_text(json.dumps(aggregate,separators=(',',':'))+'\n');(d/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
        if (i+1)%5==0:print(mode,i+1,'/',len(plan['cases']),flush=True)
    check(d,m);m['rawSha256']=pairs.digest(d/'raw.json');m['caseRawSha256']={p.name:pairs.digest(p) for p in sorted(d.glob('case-[0-9][0-9].json'))};(d/'inputs.json').write_text(json.dumps(m,indent=2)+'\n')
    print(mode,'complete, all observations retained',flush=True)
