#!/usr/bin/env python3
"""Read-only Linux counter analysis. CPU seconds and RSS are not Joules/Go heap."""
import argparse
import hashlib
import json
from pathlib import Path


def fields(text):
    out = {}
    if isinstance(text, str):
        for line in text.splitlines():
            words = line.replace(':', ' ').split()
            if len(words) >= 2:
                try:
                    out[words[0]] = int(words[1])
                except ValueError:
                    pass
    return out


def proc(sample):
    text = sample.get('stat')
    if not isinstance(text, str):
        return None
    rest = text[text.rfind(')') + 2:].split()
    if len(rest) < 22:
        return None
    return (int(sample['service']['MainPID']), int(rest[19]),
            int(rest[11]) + int(rest[12]))


def delta(old, new):
    return {k: new[k] - old[k] for k in old.keys() & new.keys()
            if new[k] >= old[k]}


def host_cpu(text):
    if isinstance(text, str):
        for line in text.splitlines():
            parts = line.split()
            if parts and parts[0] == 'cpu' and len(parts) >= 9:
                # guest/guest_nice already included in user/nice: never add twice.
                return dict(zip(('user', 'nice', 'system', 'idle', 'iowait',
                                 'irq', 'softirq', 'steal'), map(int, parts[1:9])))
    return {}


def pressure(text):
    out = {}
    if isinstance(text, str):
        for line in text.splitlines():
            parts = line.split()
            for part in parts[1:]:
                if part.startswith('total='):
                    out[parts[0]] = int(part.split('=')[1])
    return out


def interval(x, y):
    dt = (y['monotonic_ns'] - x['monotonic_ns']) / 1e9
    if dt <= 0:
        raise ValueError('samples must have increasing monotonic timestamps')
    first, last = proc(x), proc(y)
    same = bool(first and last and first[:2] == last[:2])
    row = {'elapsed_seconds': dt, 'same_process': same}
    if same:
        ticks = y.get('clock_ticks')
        if ticks and ticks == x.get('clock_ticks') and last[2] >= first[2]:
            row['process_cpu_seconds'] = (last[2] - first[2]) / ticks
            row['average_cpu_cores'] = row['process_cpu_seconds'] / dt
        row['io_delta'] = delta(fields(x.get('io')), fields(y.get('io')))
    # Do not silently turn counter resets into a zero-usage assertion.
    old, new = host_cpu(x.get('host_stat')), host_cpu(y.get('host_stat'))
    if old and old.keys() == new.keys() and all(new[k] >= old[k] for k in old):
        d = delta(old, new)
        row['host_cpu_ticks_delta'] = d
        if sum(d.values()):
            row['host_steal_percent'] = 100 * d['steal'] / sum(d.values())
    if x.get('service', {}).get('ControlGroup') == y.get('service', {}).get('ControlGroup'):
        row['cgroup_cpu_delta'] = delta(fields(x.get('cgroup_cpu.stat')), fields(y.get('cgroup_cpu.stat')))
        for resource in ('cpu', 'io', 'memory'):
            key = 'cgroup_' + resource + '.pressure'
            d = delta(pressure(x.get(key)), pressure(y.get(key)))
            if d:
                row[resource + '_pressure_usec_delta'] = d
                row[resource + '_pressure_percent'] = {k: v / (dt * 1e4) for k, v in d.items()}
    row['rss_kib'] = fields(y.get('status')).get('VmRSS')
    return row


def analyze(samples):
    if len(samples) < 2:
        raise ValueError('at least two samples required')
    rows = [interval(x, y) for x, y in zip(samples, samples[1:])]
    whole = interval(samples[0], samples[-1])
    if not all(r['same_process'] for r in rows):
        whole = {'elapsed_seconds': whole['elapsed_seconds'], 'same_process': False,
                 'invalid_reason': 'process identity changed; inspect intervals'}
    rss = [fields(s.get('status')).get('VmRSS') for s in samples]
    rss = [v for v in rss if v is not None]
    return {'samples': len(samples), 'start_time_ns': samples[0].get('time_ns'),
            'end_time_ns': samples[-1].get('time_ns'), 'window': whole,
            'rss_kib_range': [min(rss), max(rss)] if rss else None,
            'service_cpu_max_observed': sorted({str(s.get('cgroup_cpu.max')) for s in samples}),
            'intervals': rows, 'energy_joules': None,
            'limits': ['RSS is not Go heap allocations; this cannot locate leaks or functions.',
                       'Service cpu.max does not exclude ancestor or hypervisor limits.',
                       'Host steal covers all guest vCPUs, not this process alone.',
                       'PSI and CPU time overlap; do not subtract them from process CPU.',
                       'No binary identity, finality progress or APoW attribution is established.']}


def main():
    p = argparse.ArgumentParser()
    p.add_argument('samples')
    a = p.parse_args()
    raw = Path(a.samples).read_bytes()
    result = analyze([json.loads(x) for x in raw.splitlines() if x.strip()])
    result['input_sha256'] = hashlib.sha256(raw).hexdigest()
    print(json.dumps(result, indent=2))


if __name__ == '__main__':
    main()
