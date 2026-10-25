'use client';
import { useState } from 'react';
import { Search, Gauge, Radio, Check, LoaderCircle, Server } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { AUTO, GROUP, label, type Snapshot } from '@/lib/types';
import { cn } from '@/lib/utils';

export function Nodes({snapshot,disabled,busy,select,test}:{snapshot:Snapshot|null;disabled:boolean;busy:string|null;select:(name:string)=>Promise<boolean>;test:()=>Promise<boolean>}) {
  const [query,setQuery]=useState('');
  const group=snapshot?.proxies[GROUP];
  const names=(group?.all||[]).filter(name=>name!=='DIRECT'&&label(name).toLowerCase().includes(query.trim().toLowerCase()));
  return <Card className="nodes-card">
    <CardHeader className="flex flex-row items-center justify-between gap-4"><CardTitle>选择节点 <span className="ml-2 text-xs font-normal text-muted-foreground">{snapshot?.info.nodes?`${snapshot.info.nodes} 个`:''}</span></CardTitle>
      <Button size="sm" variant="outline" onClick={()=>void test()} disabled={disabled||!group}>{busy==='test'?<LoaderCircle className="animate-spin"/>:<Gauge/>}{busy==='test'?'测速中…':'测速'}</Button>
    </CardHeader>
    <CardContent>
      <div className="relative mb-5"><Search aria-hidden="true" className="absolute left-3 top-3 size-4 text-muted-foreground"/><Input aria-label="搜索节点" value={query} onChange={e=>setQuery(e.target.value)} placeholder="搜索节点名称…" className="h-10 bg-background pl-9"/></div>
      {!group?<div className="empty-state"><div className="empty-icon"><Server/></div><h3>{snapshot?.info.needs_subscription?'从添加订阅开始':snapshot?.info.starting?'正在加载订阅':'等待节点就绪'}</h3><p>{snapshot?.info.needs_subscription?'在右侧点击「添加订阅」，粘贴你的订阅地址。':snapshot?'请检查订阅状态，成功加载后节点会显示在这里。':'正在连接本机 fly 服务器…'}</p></div>:
        <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
          {names.map(name=>{
            const proxy=snapshot?.proxies[name];const history=proxy?.history||[];const delay=history.length?history[history.length-1].delay:null;
            const selected=name===group.now;
            return <button key={name} type="button" aria-label={`选择节点：${label(name)}`} aria-pressed={selected} disabled={disabled} onClick={()=>void select(name)}
              className={cn('node-choice',selected&&'node-selected',name===AUTO&&'sm:col-span-2')} title={label(name)}>
              <span className={cn('node-indicator',selected&&'node-indicator-selected')}>{selected?<Check className="size-3"/>:name===AUTO?<Radio className="size-3"/>:null}</span>
              <span className="min-w-0 flex-1 truncate text-sm font-medium">{label(name)}</span>
              <span className={cn('shrink-0 text-xs tabular-nums text-muted-foreground',delay&&delay<300&&'text-emerald-700',delay&&delay>=300&&'text-amber-700')}>
                {name===AUTO?'自动测速':busy==='test'?'测速中':delay===null?'未测速':delay>0?`${delay} ms`:'超时'}
              </span>
            </button>;
          })}
          {!names.length&&<p className="col-span-full py-12 text-center text-sm text-muted-foreground">没有匹配的节点，试试其他名称。</p>}
        </div>}
    </CardContent>
  </Card>;
}
