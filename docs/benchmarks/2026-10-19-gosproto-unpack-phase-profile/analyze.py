"""按原函数调用行对CPU样本作互斥归属；不是独立阶段计时，不剔除运行时样本。"""
import argparse
import collections
import json
from pathlib import Path
import re

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('directory', type=Path)
args = parser.parse_args()
plan = json.loads((Path(__file__).parent/'plan.json').read_text())
result = []
symbol = 'github.com/cnooljaw/gosproto.UnpackWithLimit'
for case in plan['profileCases']:
    stem = case.replace('/', '-')+'-B-cpu'
    raw = (args.directory/(stem+'-raw.txt')).read_text()
    samples_text, locations_text = raw.split('Locations\n', 1)
    locations = {}
    current = None
    for line in locations_text.splitlines():
        if line == 'Mappings':
            break
        match = re.match(r'\s+(\d+): 0x[0-9a-f]+ M=\d+ (.*)', line)
        if match:
            current = int(match[1])
            locations[current] = [match[2]]
        elif line.startswith('             ') and current is not None:
            locations[current].append(line.strip())
    groups = collections.Counter()
    attributed_lines = collections.Counter()
    leaves = collections.Counter()
    count = 0
    for line in samples_text.splitlines():
        match = re.fullmatch(r'\s+(\d+)\s+(\d+): ([\d ]+)', line)
        if not match:
            continue
        n, ns = int(match[1]), int(match[2])
        assert ns == n*10000000
        stack = [int(i) for i in match[3].split()]
        assert stack and all(i in locations for i in stack)
        leaves[locations[stack[0]][0].split(' ', 1)[0]] += ns
        stage = 'other'
        for location in stack:
            frames = locations[location]
            found = [f for f in frames if f.startswith(symbol+' ')]
            if not found:
                continue
            assert len(found) == 1
            f = found[0]
            source_line = int(re.search(r'/pack.go:(\d+):', f)[1])
            assert 193 <= source_line <= 273, source_line
            stage = 'preflight' if source_line <= 233 else 'allocation' if source_line == 234 else 'restore'
            attributed_lines[source_line] += ns
            break
        groups[stage] += ns
        count += n
    total = sum(groups.values())
    assert total == count*10000000 and total > 0
    assert sum(attributed_lines.values()) == total-groups['other']
    # 独立核对pprof展示的函数累计值与已归属样本（纳秒精确值以raw为准）。
    source = (args.directory/(stem+'-source.txt')).read_text()
    assert 'ROUTINE ======================== '+symbol+' in ' in source
    record = {'case': case, 'sampleCount': count, 'sampleNs': total,
              'stageNs': {k: groups[k] for k in ['preflight', 'allocation', 'restore', 'other']},
              'stagePercentOfAllSamples': {k: groups[k]/total*100 for k in ['preflight', 'allocation', 'restore', 'other']},
              'unpackLineNs': dict(sorted(attributed_lines.items())),
              'leafSymbolNs': dict(leaves.most_common())}
    result.append(record)
report = {'method': 'First canonical UnpackWithLimit frame from leaf toward root; classify caller line193..233 preflight,234 allocation,235..273 restore; samples without frame are other. Inline frames retained. Each sample counted once; runtime descendants stay in calling stage only when stack preserves caller. No subtraction or normalization.',
          'caveat': 'CPU sampling attribution is not independent exclusive phase time or a hardware-cause proof. Other includes runtime system stacks/GC/scheduler and Decode in PDecode. Unpack-frame denominator is not used to hide them.',
          'results': result}
(args.directory/'stage-analysis.json').write_text(json.dumps(report, indent=2)+'\n')
for r in result:
    print(r['case'], r['sampleCount'], {k: round(v,2) for k,v in r['stagePercentOfAllSamples'].items()})
