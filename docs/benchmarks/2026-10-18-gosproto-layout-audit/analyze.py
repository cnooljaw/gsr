"""独立核对布局诊断及分配模型，生成只含可追溯原值的汇总。"""
import json
from pathlib import Path

here = Path(__file__).resolve().parent
raw = json.loads((here/'raw.json').read_text())
models = json.loads((here/'model/model.json').read_text())
by_count = {}
for result in models:
    by_count.setdefault(result['Count'], {})[result['Policy']] = result
    assert result['Calls'] == 128
    assert result['BytesPerCall'] == result['Bytes']/128
    assert result['MallocsPerCall'] == result['Mallocs']/128
assert len(raw) == 109
assert len(by_count) == 48
assert len(models) == 192
assert len({result['Name'] for result in raw}) == len(raw)
entries = 0
failures = []
allocation_tails = []
model_costs = []
for result in raw:
    trace = result['Trace']
    count = trace['Count']
    assert trace['Local'] == min(32, count)
    assert trace['ExtraLength'] == max(0, count-32)
    assert sum(trace['Histogram'].values()) == count
    current = by_count[count]['current128']
    assert current['FinalLength'] == trace['ExtraLength']
    assert current['FinalCapacity'] == trace['ExtraCapacity']
    growth = trace['Growth'] or []
    assert len(growth) == current['MallocsPerCall']
    assert growth == (result['InitialGrowth']['128'] or [])
    if trace['Error']:
        failures.append(result['Name'])
        assert len(result['Allocations']) == 1
    else:
        assert trace['Consumed'] == count
        assert len(result['Allocations']) == 3
        assert result['Allocations'][0]['Length'] == trace['Size']
        assert result['Allocations'][0]['Capacity'] == trace['Size']
    for entry in result['Allocations']:
        entries += 1
        assert entry['Calls'] == 128
        assert entry['BytesPerCall'] == entry['Bytes']/128
        assert entry['MallocsPerCall'] == entry['Mallocs']/128
        if trace['Error']:
            assert entry['Error'] == trace['Error']
            assert entry['Length'] == entry['Capacity'] == 0
            assert entry['BytesPerCall'] == current['BytesPerCall']
            assert entry['MallocsPerCall'] == current['MallocsPerCall']
        else:
            assert not entry['Error']
            assert entry['BytesPerCall'] > current['BytesPerCall']
            assert entry['MallocsPerCall'] == current['MallocsPerCall'] + 1
        if entry['BytesPerCall'] % 1 or entry['MallocsPerCall'] % 1:
            allocation_tails.append({'name': result['Name'], **entry})
for count, policies in sorted(by_count.items()):
    a, b = policies['current128'], policies['double128']
    if b['BytesPerCall'] > a['BytesPerCall'] or b['FinalCapacity'] > a['FinalCapacity']:
        model_costs.append({'count': count, 'current': a, 'doubling': b})
assert entries == 317
assert len(failures) == 5
report = {
    'cases':len(raw), 'allocationEntries':entries, 'callsPerEntry':128,
    'totalMeasuredCalls':entries*128, 'growthCounts':len(by_count), 'modelObservations':len(models),
    'totalModelCalls':len(models)*128, 'failedAdmissionCases':failures,
    'fractionalAllocationObservations':allocation_tails,
    'doublingCostsInObservedCounts':model_costs,
    'conclusion':'Current128 fits game_large in one spill; initial64 adds allocation. Longer arrays accumulate repeated growth. Doubling reduces some cumulative allocation, but has intermediate-length costs; no production adoption or CPU conclusion.'
}
(here/'summary.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(report,ensure_ascii=False,indent=2))
