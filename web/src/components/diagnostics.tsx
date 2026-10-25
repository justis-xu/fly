'use client';
import {useState} from 'react';
import {LoaderCircle,Globe2} from 'lucide-react';
import {Dialog,DialogContent,DialogHeader,DialogTitle,DialogDescription} from '@/components/ui/dialog';
import {Button} from '@/components/ui/button';
import type {useChecks} from '@/lib/use-checks';
import type {CheckResult,CheckJob} from '@/lib/types';
export const checkTime=(at?:string)=>at&&Date.parse(at)>0?new Date(at).toLocaleString('zh-CN',{month:'numeric',day:'numeric',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false}):'未开始';
function resultText(result:CheckResult){
 const text={pending:'等待测试',running:'正在测试',ok:'响应正常',restricted:'访问受限',site_error:'网站返回错误',failed:'连接失败',timeout:'连接超时',canceled:'未完成'}[result.status];
 return text+(result.http_status?` · HTTP ${result.http_status}`:'');
}
export function Diagnostics({open,setOpen,disabled,controls}:{open:boolean;setOpen:(v:boolean)=>void;disabled:boolean;controls:ReturnType<typeof useChecks>}){
 const [mode,setMode]=useState<'sites'|'browser'>('sites');
 const {job,targets,working,error,start,cancel}=controls;
 const siteJob=job&&job.mode!=='latency'?job:null;
 const running=!!job&&!job.done;
 const startTest=(value:CheckJob['mode'])=>void start(value);
 return <Dialog open={open} onOpenChange={setOpen}><DialogContent className="max-h-[88vh] overflow-y-auto sm:max-w-2xl">
  <DialogHeader><DialogTitle className="flex items-center gap-2"><Globe2 className="size-5"/>网站可达性</DialogTitle><DialogDescription>检查公开测试地址的 GET 响应头。响应正常不代表能登录、地区解锁、播放视频或访问网站全部功能。</DialogDescription></DialogHeader>
  <div className="grid grid-cols-2 gap-2"><Button variant={mode==='sites'?'default':'outline'} onClick={()=>setMode('sites')}>当前节点</Button><Button variant={mode==='browser'?'default':'outline'} onClick={()=>setMode('browser')}>浏览器分流</Button></div>
  <p className="text-xs leading-5 text-muted-foreground">{mode==='sites'?'直接通过所选实际节点测试，自动选择会在开始时固定一个节点；测试不会切换你的选择。':'通过本机代理端口测试，遵循当前规则，可能直连或使用自动选择；用于对照浏览器应走的路径。'} 每站最多 8 秒，并发最多 8 个。</p>
  <p className="text-xs leading-5 text-muted-foreground">测试地址：{(targets.length?targets.map(t=>t.name):['Google','YouTube','GitHub','ChatGPT','Bing','百度']).join('、')}。403 / 429 表示网站响应受限，需要到浏览器中确认。</p>
  {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}
  <div className="flex flex-wrap gap-2"><Button disabled={disabled||working||running} onClick={()=>startTest(mode)}>{working&&<LoaderCircle className="animate-spin"/>}{mode==='sites'?'测试当前节点网站':'测试浏览器分流'}</Button>{running&&<Button variant="outline" disabled={working} onClick={()=>void cancel()}>取消测试</Button>}</div>
  {job?.mode==='latency'&&!job.done&&<p role="status" className="text-sm">节点测速正在进行：已处理 {job.completed} / {job.results.length}。请等待或取消后再测网站。</p>}
  {siteJob&&<div className="space-y-3">
   <div className="rounded-lg border bg-slate-50 p-3 text-xs leading-6"><p>{siteJob.route==='browser'?'上次浏览器分流测试':'本次固定测试节点'}：<strong className="break-all">{siteJob.node}</strong></p><p>开始于 {checkTime(siteJob.started_at)} · 已处理 {siteJob.completed} / {siteJob.results.length}{siteJob.done?' · 已结束':' · 进行中'}</p></div>
   {siteJob.stale&&<p role="alert" className="text-sm text-amber-700">订阅或规则已变更，以下结果属于旧配置，请重新测试。</p>}
   {siteJob.cancel_reason&&<p role="status" className="text-xs text-muted-foreground">{siteJob.cancel_reason}</p>}
   <div className="grid gap-2 sm:grid-cols-2">{siteJob.results.map(result=><div key={result.name} className="rounded-lg border p-3">
    <div className="flex flex-wrap items-center justify-between gap-2"><span className="text-sm font-medium">{result.name}</span><span className={`text-xs ${siteJob.stale?'text-muted-foreground':result.status==='ok'?'text-emerald-700':['restricted','site_error'].includes(result.status)?'text-amber-700':['failed','timeout'].includes(result.status)?'text-destructive':'text-muted-foreground'}`}>{resultText(result)}</span></div>
    <p className="mt-1 text-xs text-muted-foreground">{result.delay?`${result.delay} ms · `:''}{checkTime(result.at)}{result.message?` · ${result.message}`:''}</p>
   </div>)}</div>
  </div>}
  <p className="text-xs leading-5 text-muted-foreground">若节点测试正常、浏览器分流异常，请检查规则；两者都正常而浏览器打不开，请检查 Proxy Switcher 是否选中 fly 的 Manual Proxy，并核对 127.0.0.1 和代理端口。</p>
 </DialogContent></Dialog>;
}
