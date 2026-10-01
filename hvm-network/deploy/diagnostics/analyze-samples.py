#!/usr/bin/env python3
"""Summarize comparable Linux samples; CPU-seconds are not Joules."""
import argparse,json
p=argparse.ArgumentParser();p.add_argument('samples');a=p.parse_args()
with open(a.samples) as f: samples=[json.loads(x) for x in f if x.strip()]
def fields(text):
    if not isinstance(text,str):return {}
    out={}
    for line in text.splitlines():
        words=line.replace(':',' ').split()
        if len(words)>=2:
            try:out[words[0]]=int(words[1])
            except ValueError:pass
    return out
def proc(sample):
    text=sample.get('stat')
    if not isinstance(text,str):return None
    # /proc/pid/stat comm may contain spaces and parentheses.
    rest=text[text.rfind(')')+2:].split()
    if len(rest)<22:return None
    return (int(sample['service']['MainPID']),int(rest[19]),int(rest[11])+int(rest[12]))
rows=[]
for x,y in zip(samples,samples[1:]):
    dt=(y['monotonic_ns']-x['monotonic_ns'])/1e9
    if dt<=0:continue
    first,last=proc(x),proc(y)
    row={'elapsed_seconds':dt,'same_process':bool(first and last and first[:2]==last[:2])}
    if row['same_process']:
        ticks=y.get('clock_ticks')
        if ticks:row['process_cpu_seconds']=(last[2]-first[2])/ticks;row['average_cpu_cores']=row['process_cpu_seconds']/dt
        for label,key in [('io','io'),('cgroup_cpu','cgroup_cpu.stat')]:
            old,new=fields(x.get(key)),fields(y.get(key))
            row[label+'_delta']={k:new[k]-old[k] for k in new.keys()&old.keys() if new[k]>=old[k]}
    host0,host1=fields(x.get('status')),fields(y.get('status'))
    row['rss_kib']=host1.get('VmRSS')
    rows.append(row)
print(json.dumps({'intervals':rows,'samples':len(samples),'energy_joules':None,'limits':'CPU, I/O and throttling counters support triage; they do not identify Go functions or establish energy savings.'},indent=2))
