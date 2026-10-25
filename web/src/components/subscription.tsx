'use client';
import { useEffect, useState, useRef } from 'react';
import { RefreshCw, Link2, LoaderCircle, Eye, EyeOff } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { api } from '@/lib/api';
import type { Info,SubscriptionOptions } from '@/lib/types';

interface Props {info?:Info;disabled:boolean;busy:string|null;open:boolean;setOpen:(open:boolean)=>void;save:(url:string,options?:SubscriptionOptions)=>Promise<boolean>;update:()=>Promise<boolean>}
export function Subscription({info,disabled,busy,open,setOpen,save,update}:Props) {
  const [source,setSource]=useState('');
  const [url,setURL]=useState('');
  const readRevision=useRef(0);
  const [mode,setMode]=useState<'url'|'file'>('url');
  const [downloadProxy,setDownloadProxy]=useState('');
  const [file,setFile]=useState<{name:string;content:string}|null>(null);
  const [loading,setLoading]=useState(false);
  const [loadError,setLoadError]=useState('');
  const [validation,setValidation]=useState('');
  const [visible,setVisible]=useState(false);
  useEffect(()=>{
    const controller=new AbortController();
    api<{url:string;download_proxy?:string;imported?:boolean}>('subscription',{signal:controller.signal}).then(d=>{
      if(d.imported){setSource('本地文件');return;}try{setSource(new URL(d.url).hostname);}catch{setSource('尚未设置');}
    }).catch(()=>{if(!controller.signal.aborted)setSource('读取失败');});
    return()=>controller.abort();
  },[info?.subscription?.fetched_at,info?.imported]);
  useEffect(()=>{
    if(!open)return;
    const controller=new AbortController();
    setLoading(true);setLoadError('');setValidation('');setVisible(false);setURL('');setFile(null);readRevision.current++;
    api<{url:string;download_proxy?:string;imported?:boolean}>('subscription',{signal:controller.signal}).then(d=>{setURL(d.url);setDownloadProxy(d.download_proxy||'');setMode(d.imported?'file':'url');})
      .catch(()=>{if(!controller.signal.aborted)setLoadError('读取订阅失败，请关闭后重试。');})
      .finally(()=>{if(!controller.signal.aborted)setLoading(false);});
    return()=>controller.abort();
  },[open]);
  const submit=async(event:React.FormEvent)=>{
    event.preventDefault();
    if(mode==='url'){
      try {const parsed=new URL(url.trim());if(!['http:','https:'].includes(parsed.protocol)||!parsed.hostname)throw new Error();}
      catch {setValidation('请输入有效的 http 或 https 订阅地址。');return;}
    }else if(!file){setValidation('请先选择订阅文件。');return;}
    if(downloadProxy.trim()){
      try{const parsed=new URL(downloadProxy.trim());if(!['http:','https:','socks5:','socks5h:'].includes(parsed.protocol)||!parsed.hostname||!parsed.port)throw new Error();}
      catch{setValidation('备用代理须是含端口的 http(s):// 或 socks5:// 地址。');return;}
    }
    setValidation('');
    if(await save(mode==='url'?url.trim():'',{download_proxy:downloadProxy.trim(),...(mode==='file'&&file?{content:file.content}:{})})) {
      setSource(mode==='file'?'本地文件':new URL(url.trim()).hostname);setOpen(false);
    }
  };
  const readFile=(selected?:File)=>{
    const revision=++readRevision.current;setFile(null);setValidation('');if(!selected)return;
    if(selected.size>8*1024*1024){setValidation('文件超过 8 MiB，请选择精简后的订阅。');return;}
    const reader=new FileReader();reader.onload=()=>{if(revision===readRevision.current)setFile({name:selected.name,content:String(reader.result||'')});};reader.onerror=()=>setValidation('文件读取失败，请重新选择。');reader.readAsText(selected);
  };
  const data=info?.subscription;
  const usage=data&&data.total>0?Math.min(100,data.used/data.total*100):null;
  return <>
    <Card className="sidebar-card">
      <CardHeader><CardTitle>订阅</CardTitle></CardHeader>
      <CardContent className="space-y-4">
        <dl className="subscription-data">
          <div><dt>订阅来源</dt><dd className="max-w-44 truncate" title={source}>{source||'—'}</dd></div>
          <div><dt>已用 / 总流量</dt><dd>{data&&data.total>0?`${(data.used/1073741824).toFixed(1)} / ${(data.total/1073741824).toFixed(1)} GB`:'订阅未提供'}</dd></div>
        </dl>
        {usage!==null&&<progress aria-label="订阅流量使用比例" max={100} value={usage} className="usage"/>}
        <dl className="subscription-data">
          <div><dt>到期时间</dt><dd>{data&&data.expire>0?new Date(data.expire*1000).toLocaleDateString('zh-CN'):'订阅未提供'}</dd></div>
          <div><dt>最近更新</dt><dd>{data&&data.fetched_at>0?new Date(data.fetched_at*1000).toLocaleString('zh-CN',{month:'numeric',day:'numeric',hour:'2-digit',minute:'2-digit'}):'—'}</dd></div>
        </dl>
        <div className="grid gap-2 pt-1">
          {info?.needs_subscription?<Button onClick={()=>setOpen(true)} disabled={disabled}><Link2/>添加订阅</Button>:<>
            <Button variant="outline" onClick={()=>info?.imported?setOpen(true):void update()} disabled={disabled||!info||info.starting||info.updating}>
              <RefreshCw className={busy==='update'||info?.updating?'animate-spin':''}/>{busy==='update'||info?.updating?'更新中…':info?.imported?'重新导入文件':'更新订阅'}
            </Button>
            <Button variant="ghost" size="sm" onClick={()=>setOpen(true)} disabled={disabled||!info||info.starting||info.updating}>{info?.imported?'更换订阅来源':'修改订阅地址'}</Button>
          </>}
        </div>
        <p className="text-xs leading-relaxed text-muted-foreground">使用你自己的订阅。fly 不附带节点或订阅服务。{info?.download_route&&!info?.needs_subscription&&` 最近加载方式：${info.download_route}。`}{info?.skipped?` 跳过 ${info.skipped} 个占位或无效节点。`:null}</p>
      </CardContent>
    </Card>
    <Dialog open={open} onOpenChange={value=>{if(busy!=='save')setOpen(value);}}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg" showCloseButton={busy!=='save'}>
        <DialogHeader><DialogTitle>{info?.needs_subscription?'添加订阅':info?.imported?'更换订阅':'修改订阅地址'}</DialogTitle>
          <DialogDescription>使用服务商的 Clash/Mihomo 订阅链接，或导入已下载的订阅文件。验证通过后才替换原配置，失败时保留原配置。</DialogDescription></DialogHeader>
        <form onSubmit={submit} className="space-y-4">
          <div className="flex gap-2"><Button type="button" size="sm" variant={mode==='url'?'default':'outline'} disabled={loading||busy==='save'} onClick={()=>setMode('url')}>在线链接</Button><Button type="button" size="sm" variant={mode==='file'?'default':'outline'} disabled={loading||busy==='save'} onClick={()=>setMode('file')}>本地文件</Button></div>
          {mode==='url'?<div><label htmlFor="subscription-url" className="mb-2 block text-sm font-medium">订阅地址</label>
            <div className="relative"><Input id="subscription-url" type={visible?'text':'password'} autoComplete="off" spellCheck={false} value={url} onChange={e=>setURL(e.target.value)} placeholder="https://…" disabled={loading||!!loadError||busy==='save'} className="pr-10"/>
              <Button type="button" size="icon-sm" variant="ghost" aria-label={visible?'隐藏订阅地址':'显示订阅地址'} className="absolute right-1 top-1" onClick={()=>setVisible(v=>!v)}>{visible?<EyeOff/>:<Eye/>}</Button></div>
            <p className="mt-2 text-xs leading-relaxed text-muted-foreground">订阅链接包含你的账户凭据，请勿公开分享或截图。</p>
          </div>:<div><label htmlFor="subscription-file" className="mb-2 block text-sm font-medium">订阅文件</label><Input id="subscription-file" type="file" accept=".yaml,.yml,.txt,.conf" disabled={loading||busy==='save'} onChange={event=>readFile(event.target.files?.[0])}/><p className="mt-2 text-xs leading-5 text-muted-foreground">{file?`已读取 ${file.name}。`:'选择 YAML 或文本文件，最多 8 MiB。'} 导入后使用本地副本；更新节点需要重新导入文件。</p></div>}
          <details className="rounded-lg border p-3"><summary className="cursor-pointer text-xs">下载打不开？设置备用下载代理</summary><div className="mt-3"><label htmlFor="download-proxy" className="mb-2 block text-xs font-medium">备用下载代理</label><Input id="download-proxy" type="password" autoComplete="off" placeholder="http://127.0.0.1:7890（可留空）" value={downloadProxy} onChange={e=>setDownloadProxy(e.target.value)} disabled={loading||busy==='save'}/><p className="mt-2 text-xs leading-5 text-muted-foreground">先尝试直连，失败后尝试这里已在运行的代理，再尝试 fly 当前节点。没有可用出口时，可先在能下载的环境保存订阅文件再导入。</p></div></details>
          {loading&&<p className="text-sm text-muted-foreground">正在读取…</p>}
          {(loadError||validation)&&<p role="alert" className="text-sm text-destructive">{loadError||validation}</p>}
          <div className="flex justify-end gap-2"><Button variant="outline" type="button" disabled={busy==='save'} onClick={()=>setOpen(false)}>取消</Button><Button type="submit" disabled={loading||!!loadError||busy==='save'}>{busy==='save'&&<LoaderCircle className="animate-spin"/>}{busy==='save'?'验证中，请稍候…':mode==='file'?'验证并导入':'保存并更新'}</Button></div>
        </form>
      </DialogContent>
    </Dialog>
  </>;
}
