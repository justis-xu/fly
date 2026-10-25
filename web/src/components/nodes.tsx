'use client';
import { useEffect, useState } from 'react';
import { Search, Gauge, Radio, Check, LoaderCircle, Server } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { AUTO, GROUP, label, type Snapshot } from '@/lib/types';
import { cn } from '@/lib/utils';
import {nodeMetric,sortNodes,type SortOrder} from '@/lib/node-metrics';
import type {CheckJob,CheckResult} from '@/lib/types';

export function Nodes({snapshot,disabled,busy,select,test,sites,job,latencies={},cancel}:{snapshot:Snapshot|null;disabled:boolean;busy:string|null;select:(name:string)=>Promise<boolean>;test:()=>Promise<boolean>;sites:()=>void;job:CheckJob|null;latencies?:Record<string,CheckResult>;cancel:()=>Promise<void>}) {
  const [query,setQuery]=useState('');
  const [order,setOrder]=useState<SortOrder>('subscription');
  useEffect(()=>{try{const saved=localStorage.getItem('fly.node-sort');if(['subscription','name','latency','available'].includes(saved||''))setOrder(saved as SortOrder);}catch{}},[]);
  const group=snapshot?.proxies[GROUP];
  const measuring=job?.mode==='latency'&&!job.done&&!job.stale;
  const pendingByName=Object.fromEntries(job?.results.map(result=>[result.name,result.status])||[]);
  const names=sortNodes((group?.all||[]).filter(name=>name!=='DIRECT'&&label(name).toLowerCase().includes(query.trim().toLowerCase())),order,snapshot?.proxies||{},latencies);
  return <Card className="nodes-card">
    <CardHeader className="flex flex-wrap flex-row items-center justify-between gap-3"><CardTitle>选择节点 <span className="ml-2 text-xs font-normal text-muted-foreground">{snapshot?.info.nodes?`${snapshot.info.nodes} 个`:''}</span></CardTitle>
      <div className="flex gap-2"><Button size="sm" variant="outline" onClick={sites} disabled={!group}>测网站</Button><Button size="sm" variant="outline" onClick={()=>void test()} disabled={disabled||!group||!!job&&!job.done}>{measuring?<LoaderCircle className="animate-spin"/>:<Gauge/>}{measuring?'测速中…':'测速'}</Button></div>
    </CardHeader>
    <CardContent>
      <div className="mb-4 flex flex-wrap gap-2"><select aria-label="节点排序" value={order} onChange={e=>{const value=e.target.value as SortOrder;setOrder(value);try{localStorage.setItem('fly.node-sort',value);}catch{}}} className="h-10 rounded-lg border bg-background px-3 text-xs"><option value="subscription">订阅顺序</option><option value="name">按名称</option><option value="latency">延迟从低到高</option><option value="available">可用在前</option></select><div className="relative min-w-40 flex-1"><Search aria-hidden="true" className="absolute left-3 top-3 size-4 text-muted-foreground"/><Input aria-label="搜索节点" value={query} onChange={e=>setQuery(e.target.value)} placeholder="搜索节点名称…" className="h-10 bg-background pl-9"/></div></div>
      <p className="mb-4 text-xs leading-5 text-muted-foreground">测速检查 Google 204 响应，测的是延迟。常用网站能否访问，请点「测网站」。</p>
      {measuring&&<div className="mb-4 flex items-center justify-between gap-2 rounded-lg bg-blue-50 px-3 py-2 text-xs" role="status">已处理 {job.completed} / {job.results.length}，可随时取消<Button size="sm" variant="ghost" onClick={()=>void cancel()}>取消测速</Button></div>}
      {job?.mode==='latency'&&job.done&&<p role="status" className="mb-4 text-xs leading-5 text-muted-foreground">{job.stale?'配置已变更，请重新测速。':job.cancel_reason||`本次测速已结束：${job.results.filter(r=>r.status==='ok').length} 个通过，${job.results.filter(r=>['failed','timeout'].includes(r.status)).length} 个未通过。网站访问情况需单独测试。`}</p>}
      {!group?<div className="empty-state"><div className="empty-icon"><Server/></div><h3>{snapshot?.info.needs_subscription?'从添加订阅开始':snapshot?.info.starting?'正在加载订阅':'等待节点就绪'}</h3><p>{snapshot?.info.needs_subscription?'在「订阅」区域点击「添加订阅」，粘贴链接或导入文件。':snapshot?'请检查订阅状态，成功加载后节点会显示在这里。':'正在连接本机 fly 服务器…'}</p></div>:
        <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
          {names.map(name=>{
            const metric=nodeMetric(name,snapshot?.proxies||{},latencies);const delay=metric.delay;
            const pending=measuring?pendingByName[name]:null;
            const selected=name===group.now;
            return <button key={name} type="button" aria-label={`选择节点：${label(name)}`} aria-pressed={selected} disabled={disabled} onClick={()=>void select(name)}
              className={cn('node-choice',selected&&'node-selected',name===AUTO&&'sm:col-span-2')} title={`${label(name)}${metric.at?` · ${metric.source==='manual'?'手动测速':'自动检测'} ${new Date(metric.at).toLocaleString('zh-CN')}`:''}`}>
              <span className={cn('node-indicator',selected&&'node-indicator-selected')}>{selected?<Check className="size-3"/>:name===AUTO?<Radio className="size-3"/>:null}</span>
              <span className="min-w-0 flex-1 truncate text-sm font-medium">{label(name)}</span>
              <span className={cn('shrink-0 text-xs tabular-nums text-muted-foreground',delay&&delay<300&&'text-emerald-700',delay&&delay>=300&&'text-amber-700')}>
                {name===AUTO?'自动检测':pending==='running'?'测量中':pending==='pending'?'等待':metric.status==='ok'?`${delay} ms`:metric.status==='untested'?'未测速':metric.status==='timeout'?'超时':'未通过'}
              </span>
            </button>;
          })}
          {!names.length&&<p className="col-span-full py-12 text-center text-sm text-muted-foreground">没有匹配的节点，试试其他名称。</p>}
        </div>}
    </CardContent>
  </Card>;
}
