"""逐块独立复算280个控制/比较进程，保留失败、分配尾差和全部GC事件；不作噪声扣除。"""
import argparse
import json
import math
from pathlib import Path
import statistics

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('directory', type=Path)
options = parser.parse_args()
directory = options.directory.resolve()
here = Path(__file__).resolve().parent
plan = json.loads((here/'plan.json').read_text())
counts = {'processes': 0, 'blocks': 0, 'quartets': 0, 'rounds': 0, 'snapshots': 0}
joined = {}
summary = {}
for mode in plan['modes']:
    raw = json.loads((directory/mode/'raw.json').read_text())
    supplied = json.loads((directory/mode/'summary.json').read_text())
    compare = mode.startswith('pass')
    assert [r['name'] for r in raw['results']] == plan['cases']
    assert [r['name'] for r in supplied] == plan['cases']
    assert (raw['rounds'], raw['quartetsPerRound'], raw['targetBlockNs'], raw['gogcPercent']) == (6, 4, 10000000, 100)
    rows = []
    for result, expected in zip(raw['results'], supplied):
        logs = [[] for _ in range(6)]
        ns = {'A': 0, 'B': 0}
        calls = {'A': 0, 'B': 0}
        block_gcs = []
        assert len(result['quartets']) == 24
        for index, quartet in enumerate(result['quartets']):
            r, q = divmod(index, 4)
            assert (quartet['round'], quartet['index']) == (r, q)
            order = 'ABBA' if (r+q) % 2 == 0 else 'BAAB'
            assert ''.join(b['arm'] for b in quartet['blocks']) == order
            value = 0
            for block in quartet['blocks']:
                arm = block['arm']
                assert block['calls'] == result['callsPerBlock'] and block['calls'] > 0 and block['nanoseconds'] > 0
                ns[arm] += block['nanoseconds']
                calls[arm] += block['calls']
                value += (1 if arm == 'B' else -1)*math.log(block['nanoseconds']/block['calls'])
                block_gcs.append(block['completedGcCycles'])
                counts['blocks'] += 1
            ratio = math.exp(value/2)
            assert math.isclose(ratio, quartet['rightOverLeft'], rel_tol=1e-12)
            logs[r].append(math.log(ratio))
            counts['quartets'] += 1
        ratios = [math.exp(sum(v)/4) for v in logs]
        assert len(result['roundRatios']) == 6
        assert all(math.isclose(a, b, rel_tol=1e-12) for a, b in zip(ratios, result['roundRatios']))
        median = statistics.median(ratios)
        quantiles = statistics.quantiles(ratios, n=4, method='inclusive')
        iqr = quantiles[2]-quantiles[0]
        within = sum(abs(r-1) <= 0.05 for r in ratios)
        allocation = {}
        for arm, key in [('A', 'leftNsPerOp'), ('B', 'rightNsPerOp')]:
            assert math.isclose(ns[arm]/calls[arm], result[key], rel_tol=1e-12)
            observations = [a for a in result['allocations'] if a['arm'] == arm]
            assert len(observations) == 3 and all(a['calls'] == 128 for a in observations)
            allocation[arm] = {k: statistics.median(a[k]/a['calls'] for a in observations) for k in ['bytes', 'mallocs']}
        same = allocation['A'] == allocation['B']
        timing = abs(median-1) <= 0.02 and iqr <= 0.03 and within >= 5
        assert math.isclose(median, expected['medianRatio'], rel_tol=1e-12)
        assert math.isclose(iqr, expected['ratioIQR'], abs_tol=1e-12)
        assert (within, allocation, same) == (expected['roundsWithin5Pct'], expected['allocationMedian'], expected['sameAllocation'])
        if compare:
            assert expected['timingAccepted'] is None and expected['controlAccepted'] is None
        else:
            assert (timing, timing and same) == (expected['timingAccepted'], expected['controlAccepted'])
        if mode == 'self':
            assert result['sameFunction']
        telemetry = result['telemetry']
        assert [s['Stage'] for s in telemetry] == plan['telemetry']
        assert all(s['GOGC'] == 100 and s['MemoryLimit'] == 2**63-1 for s in telemetry)
        assert all(a['GC'] <= b['GC'] and a['Limiter'] <= b['Limiter'] and a['AllocBytes'] <= b['AllocBytes'] for a, b in zip(telemetry, telemetry[1:]))
        limiter_intervals = [b['Stage'] for a, b in zip(telemetry, telemetry[1:]) if b['Limiter'] > a['Limiter']]
        row = {**expected, 'leftNsPerOp': result['leftNsPerOp'], 'rightNsPerOp': result['rightNsPerOp'], 'cumulativeDeltaNs': result['rightNsPerOp']-result['leftNsPerOp'], 'cumulativeRatio': result['rightNsPerOp']/result['leftNsPerOp'], 'initialLimiter': telemetry[0]['Limiter'], 'finalLimiter': telemetry[-1]['Limiter'], 'limiterEnabledDuringMeasuredInterval': 'after_timing' in limiter_intervals, 'limiterChangeIntervals': limiter_intervals, 'initialHeap': telemetry[0]['HeapObjects'], 'largestSampledHeap': max(s['HeapObjects'] for s in telemetry), 'initialScanHeap': telemetry[0]['HeapScan'], 'blockGCMinMax': [min(block_gcs), max(block_gcs)], 'zeroCompletedGCBlocks': block_gcs.count(0)}
        deltas = []
        for r in range(6):
            arm_logs = {'A': [], 'B': []}
            for quartet in result['quartets'][r*4:r*4+4]:
                for block in quartet['blocks']:
                    arm_logs[block['arm']].append(math.log(block['nanoseconds']/block['calls']))
            deltas.append(math.exp(statistics.mean(arm_logs['B']))-math.exp(statistics.mean(arm_logs['A'])))
        row['roundGeometricDeltaNs'] = deltas
        row['medianRoundGeometricDeltaNs'] = statistics.median(deltas)
        rows.append(row)
        joined.setdefault(result['name'], {'name': result['name']})[mode] = row
        counts['processes'] += 1
        counts['rounds'] += 6
        counts['snapshots'] += 4
    summary[mode] = {
        'controlAccepted': None if compare else sum(r['controlAccepted'] for r in rows),
        'sameAllocation': sum(r['sameAllocation'] for r in rows),
        'initialLimiterNonzero': sum(r['initialLimiter'] != 0 for r in rows),
        'anyLimiterChange': sum(bool(r['limiterChangeIntervals']) for r in rows),
        'largestSampledHeap': max(r['largestSampledHeap'] for r in rows),
        'unequalAllocations': [r['name'] for r in rows if not r['sameAllocation']],
        'failedControls': None if compare else [r['name'] for r in rows if not r['controlAccepted']],
    }
assert counts == {'processes': 280, 'blocks': 26880, 'quartets': 6720, 'rounds': 1680, 'snapshots': 1120}
report = {'scope': 'Independent paired/cumulative recalculation of current70 per-case processes;retain all failed controls,outliers and allocation tails;no subtraction or minimum gain threshold', 'counts': counts, 'relativeTolerance': 1e-12, 'modes': summary, 'results': list(joined.values())}
(directory/'analysis.json').write_text(json.dumps(report, indent=2)+'\n')
print(json.dumps({'counts': counts, 'modes': summary}, indent=2))
