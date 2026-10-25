'use client';
import { useEffect, useState } from 'react';
import { RefreshCw, Link2, LoaderCircle, Eye, EyeOff } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { api } from '@/lib/api';
import type { Info } from '@/lib/types';

interface Props {info?:Info;disabled:boolean;busy:string|null;open:boolean;setOpen:(open:boolean)=>void;save:(url:string)=>Promise<boolean>;update:()=>Promise<boolean>}
export function Subscription({info,disabled,busy,open,setOpen,save,update}:Props) {
  const [source,setSource]=useState('');
  const [url,setURL]=useState('');
  const [loading,setLoading]=useState(false);
  const [loadError,setLoadError]=useState('');
  const [validation,setValidation]=useState('');
  const [visible,setVisible]=useState(false);
  useEffect(()=>{
    const controller=new AbortController();
    api<{url:string}>('subscription',{signal:controller.signal}).then(d=>{
      try{setSource(new URL(d.url).hostname);}catch{setSource('尚未设置');}
    }).catch(()=>{if(!controller.signal.aborted)setSource('读取失败');});
    return()=>controller.abort();
  },[]);
  useEffect(()=>{
    if(!open)return;
    const controller=new AbortController();
    setLoading(true);setLoadError('');setValidation('');setVisible(false);setURL('');
    api<{url:string}>('subscription',{signal:controller.signal}).then(d=>{setURL(d.url);})
      .catch(()=>{if(!controller.signal.aborted)setLoadError('读取订阅失败，请关闭后重试。');})
      .finally(()=>{if(!controller.signal.aborted)setLoading(false);});
    return()=>controller.abort();
  },[open]);
  const submit=async(event:React.FormEvent)=>{
    event.preventDefault();
    try {const parsed=new URL(url.trim());if(!['http:','https:'].includes(parsed.protocol)||!parsed.hostname)throw new Error();}
    catch {setValidation('请输入有效的 http 或 https 订阅地址。');return;}
    setValidation('');
    if(await save(url.trim())) {setSource(new URL(url.trim()).hostname);setOpen(false);}
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
            <Button variant="outline" onClick={()=>void update()} disabled={disabled||!info||info.starting||info.updating}>
              <RefreshCw className={busy==='update'||info?.updating?'animate-spin':''}/>{busy==='update'||info?.updating?'更新中…':'更新订阅'}
            </Button>
            <Button variant="ghost" size="sm" onClick={()=>setOpen(true)} disabled={disabled||!info||info.starting||info.updating}>修改订阅地址</Button>
          </>}
        </div>
        <p className="text-xs leading-relaxed text-muted-foreground">使用你自己的订阅。fly 不附带节点或订阅服务。</p>
      </CardContent>
    </Card>
    <Dialog open={open} onOpenChange={value=>{if(busy!=='save')setOpen(value);}}>
      <DialogContent className="sm:max-w-lg" showCloseButton={busy!=='save'}>
        <DialogHeader><DialogTitle>{info?.needs_subscription?'添加订阅':'修改订阅地址'}</DialogTitle>
          <DialogDescription>粘贴服务商提供的订阅链接。保存后自动加载节点和分流规则。</DialogDescription></DialogHeader>
        <form onSubmit={submit} className="space-y-4">
          <div><label htmlFor="subscription-url" className="mb-2 block text-sm font-medium">订阅地址</label>
            <div className="relative"><Input id="subscription-url" type={visible?'text':'password'} autoComplete="off" spellCheck={false} value={url} onChange={e=>setURL(e.target.value)} placeholder="https://…" disabled={loading||!!loadError||busy==='save'} className="pr-10"/>
              <Button type="button" size="icon-sm" variant="ghost" aria-label={visible?'隐藏订阅地址':'显示订阅地址'} className="absolute right-1 top-1" onClick={()=>setVisible(v=>!v)}>{visible?<EyeOff/>:<Eye/>}</Button></div>
            <p className="mt-2 text-xs leading-relaxed text-muted-foreground">订阅链接包含你的账户凭据，请勿公开分享或截图。</p>
          </div>
          {loading&&<p className="text-sm text-muted-foreground">正在读取…</p>}
          {(loadError||validation)&&<p role="alert" className="text-sm text-destructive">{loadError||validation}</p>}
          <div className="flex justify-end gap-2"><Button variant="outline" type="button" disabled={busy==='save'} onClick={()=>setOpen(false)}>取消</Button><Button type="submit" disabled={loading||!!loadError||busy==='save'}>{busy==='save'&&<LoaderCircle className="animate-spin"/>}{busy==='save'?'保存中…':'保存并更新'}</Button></div>
        </form>
      </DialogContent>
    </Dialog>
  </>;
}
