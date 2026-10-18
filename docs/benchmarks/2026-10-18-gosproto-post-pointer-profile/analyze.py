"""从保存的 pprof 文本和原命令整理归因；不产生 A/B 或插桩速度排名。"""

import argparse
import json
from pathlib import Path
import re

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('directory', type=Path)
options = parser.parse_args()
root = options.directory.resolve()
plan = json.loads((root / 'plan.json').read_text())
commands = json.loads((root / 'commands.json').read_text())
pattern = re.compile(r'^\s+\S+\s+([\d.]+)%\s+[\d.]+%\s+\S+\s+([\d.]+)%\s+(.+)$')
benchmark = re.compile(r'^(BenchmarkCurrentEncoding/\S+)\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op$', re.M)
rows = []
for case in plan['cases']:
    for entry in plan['entries']:
        name = case + '-' + entry
        functions = {}
        cpu = (root / (name + '-cpu-cum.txt')).read_text()
        for line in cpu.splitlines():
            match = pattern.match(line)
            if match:
                label = match[3].removesuffix(' (inline)')
                assert label not in functions
                functions[label] = {'flatPct': float(match[1]), 'cumPct': float(match[2]), 'inline': match[3].endswith(' (inline)')}
        assert len(functions) == 40, (name, len(functions))
        runs = {}
        for command in commands:
            role = next((role for role, suffix in [('cpu', '.cpu'), ('memory', '.mem')] if any(arg.endswith(name + suffix) for arg in command['args']) and any(arg.startswith('-test.bench=') for arg in command['args'])), None)
            if role is None:
                continue
            matches = list(benchmark.finditer(command['output']))
            assert len(matches) == 1 and matches[0][1] == 'BenchmarkCurrentEncoding/' + case + '/' + entry
            match = matches[0]
            assert role not in runs
            runs[role] = {'iterations': int(match[2]), 'instrumentedNsPerOp': float(match[3]), 'bytesPerOp': int(match[4]), 'allocsPerOp': int(match[5])}
        assert set(runs) == {'cpu', 'memory'}
        assert runs['memory']['iterations'] == 20000
        sample = re.search(r'Total samples = ([\d.]+)(\w+)', cpu)
        assert sample
        rows.append({'case': case, 'entry': entry, 'totalCpuSampleTime': sample.group(0).removeprefix('Total samples = '), 'functionsInTop40': functions, 'processes': runs})
summary = {'scope': 'read-only diagnosis; profile percentages are not speedup predictions, instrumented timings are not paired baselines', 'cpuDenominator': 'all process CPU samples; parent/child cumulative rows overlap; absent top40 row is not zero', 'rows': rows}
(root / 'summary.json').write_text(json.dumps(summary, indent=2) + '\n')
for row in rows:
    print(row['case'], row['entry'], row['processes']['memory']['bytesPerOp'], row['processes']['memory']['allocsPerOp'])
