'use client';
import {useEffect,useState} from 'react';
import {ArrowUp,ArrowDown,Plus,Trash2,ShieldCheck,Search} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Card,CardHeader,CardTitle,CardContent} from '@/components/ui/card';
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogDescription} from '@/components/ui/dialog';
import {api,jsonBody,message} from '@/lib/api';
import {label,type RuleData,type UserRule} from '@/lib/types';
const selectStyle='h-9 rounded-md border bg-background px-2 text-xs';
const actionLabel={proxy:'代理',direct:'直连',reject:'拒绝'};
interface Page {items:string[];offset:number;total:number;more:boolean}
export function Routing({routing,disabled,changed}:{routing?:RuleData;disabled:boolean;changed:()=>Promise<void>}){
 const [open,setOpen]=useState(false),[tab,setTab]=useState<'custom'|'subscription'|'providers'>('custom');
 const [rules,setRules]=useState<UserRule[]>([]),[loaded,setLoaded]=useState(false),[saving,setSaving]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const [query,setQuery]=useState(''),[shown,setShown]=useState(200),[host,setHost]=useState(''),[explanation,setExplanation]=useState('');
 const [provider,setProvider]=useState(''),[providerQuery,setProviderQuery]=useState(''),[searchTerm,setSearchTerm]=useState(''),[offset,setOffset]=useState(0),[page,setPage]=useState<Page|null>(null),[providerError,setProviderError]=useState('');
 useEffect(()=>{
  if(!open)return;const controller=new AbortController();setLoaded(false);setError('');setNotice('');setTab('custom');setQuery('');setShown(200);setExplanation('');
  api<{user_rules:UserRule[]}>('routing',{signal:controller.signal}).then(d=>{setRules(d.user_rules||[]);setLoaded(true);}).catch(e=>{if(!controller.signal.aborted)setError(message(e));});
  return()=>controller.abort();
 },[open]);
 useEffect(()=>{
  if(!open||!provider)return;const controller=new AbortController();setPage(null);setProviderError('');
  const parameters=new URLSearchParams({q:searchTerm,offset:String(offset),limit:'100'});
  api<Page>(`rule-providers/${encodeURIComponent(provider)}?${parameters}`,{signal:controller.signal}).then(setPage).catch(e=>{if(!controller.signal.aborted)setProviderError(message(e));});
  return()=>controller.abort();
 },[open,provider,searchTerm,offset]);
 const edit=(index:number,change:Partial<UserRule>)=>{setRules(previous=>previous.map((r,i)=>i===index?{...r,...change}:r));setNotice('');setExplanation('');};
 const move=(index:number,direction:number)=>{setRules(previous=>{const next=[...previous];[next[index],next[index+direction]]=[next[index+direction],next[index]];return next;});setNotice('');setExplanation('');};
 const save=async()=>{
  setSaving(true);setError('');setNotice('');
  try{await api('routing',{method:'PUT',...jsonBody({user_rules:rules})});const accepted=await api<{user_rules:UserRule[]}>('routing');setRules(accepted.user_rules);setNotice('规则已保存并生效。');await changed();}
  catch(e){setError(message(e));}finally{setSaving(false);}
 };
 const explain=async()=>{
  try{const d=await api<{matched_index:number;user_rules:UserRule[]}>(`routing?host=${encodeURIComponent(host)}`);const rule=d.user_rules[d.matched_index];setExplanation(rule?`命中已保存的第 ${d.matched_index+1} 条用户规则 → ${actionLabel[rule.action]}`:'未命中用户规则，将继续匹配订阅或内置规则。IP 网段对域名的匹配还取决于 DNS，请用浏览器分流测试确认。');}
  catch(e){setExplanation(message(e));}
 };
 const visible=(routing?.rules||[]).filter(r=>`${r.type} ${r.payload} ${r.target}`.toLowerCase().includes(query.trim().toLowerCase()));
 const pending=routing?.providers.filter(p=>p.count===0).length||0;
 return <>
  <Card className="sidebar-card"><CardHeader><CardTitle className="flex items-center gap-2"><ShieldCheck className="size-4 text-muted-foreground"/>分流规则</CardTitle></CardHeader><CardContent className="space-y-3">
   <p className="text-xs leading-5 text-muted-foreground">{!routing?'等待服务器连接':routing.providers.length?`${routing.providers.length} 个订阅规则集${pending?`，${pending} 个暂无有效条目`:'已加载'}`:routing.rules.length?'使用订阅或内置分流规则':'添加订阅后加载分流规则'}。可添加自己的域名和 IP 规则。</p>
   <Button size="sm" variant="outline" className="w-full" onClick={()=>setOpen(true)} disabled={disabled}>查看 / 修改规则</Button>
  </CardContent></Card>
  <Dialog open={open} onOpenChange={v=>{if(!saving)setOpen(v);}}><DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-3xl" showCloseButton={!saving}>
   <DialogHeader><DialogTitle>分流规则</DialogTitle><DialogDescription>从上到下先匹配先执行。用户规则优先于订阅规则，更新订阅后保留。只影响经过 fly 的流量。</DialogDescription></DialogHeader>
   <div className="flex flex-wrap gap-2">{([['custom','自定义规则'],['subscription','订阅规则'],['providers','规则集内容']] as const).map(([value,text])=><Button key={value} size="sm" variant={tab===value?'default':'outline'} onClick={()=>setTab(value)}>{text}</Button>)}</div>
   {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}{notice&&<p role="status" className="text-sm text-emerald-700">{notice}</p>}
   {tab==='custom'&&<div className="space-y-4">
    {!loaded&&!error&&<p className="text-sm text-muted-foreground">正在读取规则…</p>}
    {loaded&&<>
     {!rules.length&&<p className="rounded-lg border border-dashed p-5 text-sm text-muted-foreground">还没有自定义规则。添加一条，例如让 example.org 及其子域名始终直连。</p>}
     <div className="space-y-2">{rules.map((rule,index)=><div key={index} className="rounded-lg border bg-slate-50/60 p-3">
      <div className="mb-2 flex items-center justify-between"><span className="text-xs text-muted-foreground">第 {index+1} 条</span><div className="flex gap-1">
       <Button size="icon-sm" variant="ghost" aria-label={`上移第 ${index+1} 条规则`} disabled={saving||index===0} onClick={()=>move(index,-1)}><ArrowUp/></Button>
       <Button size="icon-sm" variant="ghost" aria-label={`下移第 ${index+1} 条规则`} disabled={saving||index===rules.length-1} onClick={()=>move(index,1)}><ArrowDown/></Button>
       <Button size="icon-sm" variant="ghost" aria-label={`删除第 ${index+1} 条规则`} disabled={saving} onClick={()=>{setRules(previous=>previous.filter((_,i)=>i!==index));setNotice('');}}><Trash2/></Button>
      </div></div>
      <div className="grid gap-2 sm:grid-cols-[140px_1fr_90px]">
       <select aria-label={`第 ${index+1} 条规则类型`} className={selectStyle} value={rule.match} disabled={saving} onChange={e=>edit(index,{match:e.target.value as UserRule['match']})}><option value="domain_suffix">域名及子域名</option><option value="domain">仅此域名</option><option value="ip_cidr">IP 网段</option></select>
       <Input aria-label={`第 ${index+1} 条规则目标`} value={rule.value} placeholder={rule.match==='ip_cidr'?'192.168.1.0/24':'example.org'} disabled={saving} onChange={e=>edit(index,{value:e.target.value})}/>
       <select aria-label={`第 ${index+1} 条规则动作`} className={selectStyle} value={rule.action} disabled={saving} onChange={e=>edit(index,{action:e.target.value as UserRule['action']})}><option value="proxy">代理</option><option value="direct">直连</option><option value="reject">拒绝</option></select>
      </div>
     </div>)}</div>
     <div className="flex flex-wrap justify-between gap-2"><Button size="sm" variant="outline" disabled={saving||rules.length>=200} onClick={()=>{setRules(previous=>[...previous,{match:'domain_suffix',value:'',action:'proxy'}]);setNotice('');}}><Plus/>添加规则</Button><Button size="sm" disabled={saving} onClick={()=>void save()}>{saving?'保存中…':'保存规则'}</Button></div>
     <p className="text-xs leading-5 text-muted-foreground">先编辑，再点「保存规则」才会生效。直连：使用本机网络；代理：使用所选节点；拒绝：阻止连接。上移可提高优先级。IP 网段需带 / 前缀长度。</p>
     <div className="rounded-lg border p-3"><label htmlFor="rule-host" className="mb-2 block text-xs font-medium">检查已保存的用户规则</label><div className="flex gap-2"><Input id="rule-host" value={host} placeholder="输入域名、IP 或网址" onChange={e=>setHost(e.target.value)}/><Button size="sm" variant="outline" disabled={!host.trim()} onClick={()=>void explain()}>检查</Button></div>{explanation&&<p className="mt-2 text-xs leading-5" role="status">{explanation}</p>}</div>
    </>}
   </div>}
   {tab==='subscription'&&<div className="space-y-3"><Input aria-label="搜索订阅规则" value={query} onChange={e=>{setQuery(e.target.value);setShown(200);}} placeholder="搜索域名、规则类型或去向…"/>
    <p className="text-xs text-muted-foreground">{visible.length} 条匹配。这里查看实际顶层规则（含优先执行的用户规则）；规则集内的条目请切换到「规则集内容」。</p>
    <div className="max-h-80 overflow-y-auto rounded-lg border p-3 text-xs">{visible.slice(0,shown).map((rule,index)=><div key={index} className="rule-item break-all"><span className="font-medium">{rule.type}</span> {rule.payload} <span className="text-muted-foreground">→ {label(rule.target)}</span></div>)}{!visible.length&&<p className="py-4 text-muted-foreground">没有匹配规则。</p>}</div>
    {visible.length>shown&&<Button size="sm" variant="outline" onClick={()=>setShown(n=>n+200)}>再显示 200 条</Button>}
   </div>}
   {tab==='providers'&&<div className="space-y-3">
    <div className="flex flex-wrap gap-2">{routing?.providers.map(p=><Button key={p.name} variant={provider===p.name?'default':'outline'} size="sm" onClick={()=>{setProvider(p.name);setProviderQuery('');setSearchTerm('');setOffset(0);}}>{p.name} · {p.count} 条</Button>)}</div>
    {!routing?.providers.length?<p className="text-sm text-muted-foreground">此订阅没有外部规则集。</p>:!provider?<p className="text-sm text-muted-foreground">选择上方规则集，查看内容。</p>:<>
     <form className="flex gap-2" onSubmit={event=>{event.preventDefault();setSearchTerm(providerQuery);setOffset(0);}}><Input aria-label="搜索规则集内容" value={providerQuery} onChange={e=>setProviderQuery(e.target.value)} placeholder="搜索域名或 IP…"/><Button size="sm" variant="outline" type="submit"><Search/>搜索</Button></form>
     {providerError&&<p role="alert" className="text-sm text-destructive">{providerError}</p>}
     {!page&&!providerError?<p className="text-sm text-muted-foreground">正在读取规则资源…</p>:page&&<><p className="text-xs text-muted-foreground">匹配 {page.total} 条，本页最多 100 条。资源随「更新订阅」一起更新；这里显示已下载的规则缓存。</p><div className="max-h-72 overflow-y-auto rounded-lg border p-3 text-xs">{page.items.map((item,index)=><div key={index} className="rule-item break-all">{item}</div>)}{!page.items.length&&<p>没有匹配条目。</p>}</div><div className="flex justify-between"><Button size="sm" variant="outline" disabled={offset===0} onClick={()=>setOffset(v=>Math.max(0,v-100))}>上一页</Button><Button size="sm" variant="outline" disabled={!page.more} onClick={()=>setOffset(v=>v+100)}>下一页</Button></div></>}
    </>}
   </div>}
  </DialogContent></Dialog>
 </>;
}
