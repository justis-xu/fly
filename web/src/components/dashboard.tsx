'use client';
import { useState } from 'react';
import { ArrowUpRight, HelpCircle, Copy, Check, CircleAlert, X, Network } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {Routing} from './routing';
import {Diagnostics} from './diagnostics';
import {useChecks} from '@/lib/use-checks';
import {nodeMetric} from '@/lib/node-metrics';
import { Nodes } from './nodes';
import { Subscription } from './subscription';
import { SetupSteps, SetupHelp } from './setup-help';
import {redact} from '@/lib/api';
import { useDashboard } from '@/lib/use-dashboard';
import { AUTO, GROUP, label } from '@/lib/types';

export default function Dashboard() {
  const {snapshot,connectionError,operation,busy,dismiss,select,update,save,refresh}=useDashboard();
  const [subscriptionOpen,setSubscriptionOpen]=useState(false);
  const [helpOpen,setHelpOpen]=useState(false);
  const [checksOpen,setChecksOpen]=useState(false);
  const checks=useChecks(snapshot?.info.generation);
  const [copied,setCopied]=useState(false);
  const [copyError,setCopyError]=useState('');
  const info=snapshot?.info;
  const group=snapshot?.proxies[GROUP];
  const proxyAddress=info?`127.0.0.1:${info.proxy_port}`:'';
  const status=connectionError?'连接中断':!info?'正在连接':info.starting?'正在启动':info.updating?'更新订阅中':info.needs_subscription?'等待添加订阅':info.ready?'本机代理已启动':'代理未就绪';
  const backendMessage=info?.update_err?`未启用新订阅：${redact(info.update_err)}`:info?.last_err?`${info.ready?'继续使用已保留的配置：':'订阅加载失败：'}${redact(info.last_err)}`:
    info?.updating?'正在拉取订阅，节点较多时请稍候…':info?.starting?'正在加载订阅和分流规则，请稍候…':'';
  const routing=snapshot?.routing;
  const chosen=group?.now===AUTO?snapshot?.proxies[AUTO]?.now:group?.now;
  const health=chosen?nodeMetric(chosen,snapshot?.proxies||{},checks.latencies):null;
  const copy=async()=>{
    try{await navigator.clipboard.writeText(proxyAddress);setCopied(true);setCopyError('');}
    catch{setCopyError('复制失败，请手动复制代理地址。');}
  };
  const disabled=!!busy||!!connectionError;
  return <main className="dashboard">
    <header className="page-header"><div className="flex items-center gap-3"><span className="logo-mark"><ArrowUpRight className="size-6" strokeWidth={2}/></span><div className="flex items-baseline gap-3"><span className="logo-word">fly</span><span className="hidden text-xs text-muted-foreground sm:inline">本地代理服务器</span></div></div>
      <div className="flex items-center gap-3"><Badge variant="outline" className={`service-badge ${info?.ready&&!connectionError?'is-ready':''}`}><span/>{status}</Badge><Button variant="ghost" size="sm" aria-label="使用帮助" onClick={()=>setHelpOpen(true)}><HelpCircle/><span className="hidden sm:inline">使用帮助</span></Button></div>
    </header>
    <div className="mb-5 space-y-3" aria-live="polite">
      {connectionError&&<Alert variant="destructive"><CircleAlert/><AlertDescription>{connectionError}</AlertDescription></Alert>}
      {operation&&<Alert variant={operation.error?'destructive':'default'} className="relative pr-12"><CircleAlert/><AlertDescription>{operation.text}</AlertDescription><Button variant="ghost" size="icon-sm" className="absolute right-2 top-2" aria-label="关闭操作提示" onClick={dismiss}><X/></Button></Alert>}
      {backendMessage&&<Alert variant={info?.update_err||info?.last_err?'destructive':'default'}><CircleAlert/><AlertDescription>{backendMessage}</AlertDescription></Alert>}
      {checks.error&&<Alert variant="destructive"><CircleAlert/><AlertDescription>{checks.error}</AlertDescription></Alert>}
      {copyError&&<Alert variant="destructive"><CircleAlert/><AlertDescription>{copyError}</AlertDescription></Alert>}
    </div>
    <Card className="connection-card"><CardContent className="flex flex-col gap-5 md:flex-row md:items-center md:justify-between">
      <div className="min-w-0"><p className="eyebrow">当前使用</p><h1 className="current-node">{group?.now?label(group.now):info?.needs_subscription?'请先添加订阅':info?.starting?'正在加载…':info?'代理尚未就绪':'正在连接…'}</h1><p className="mt-2 text-xs text-muted-foreground">{group?.now===AUTO?`自动选择 · ${snapshot?.proxies[AUTO]?.now||'正在检测可用节点'}`:group?'订阅分流 · 代理流量使用此节点':'添加自己的订阅，然后通过 Proxy Switcher 使用'}</p>{group&&<p className="mt-2 text-xs text-muted-foreground">{health?.status==='ok'?`最近${health.source==='manual'?'手动测速':'自动检测'}通过（${health.delay} ms），常用网站需单独测试。`:health?.status==='untested'?'节点尚无检测记录，请点「测网站」确认能否访问。':'最近延迟目标未通过，请换节点或测试常用网站确认。'}{health?.at&&` ${new Date(health.at).toLocaleString('zh-CN',{month:'numeric',day:'numeric',hour:'2-digit',minute:'2-digit',hour12:false})}`}</p>}{health&&['failed','timeout'].includes(health.status)&&group?.now!==AUTO&&<Button size="sm" variant="outline" className="mt-3" disabled={!!busy||!!connectionError} onClick={()=>void select(AUTO)}>尝试自动选择</Button>}</div>
      <div className="proxy-address"><p className="eyebrow">Proxy Switcher · HTTP / SOCKS5</p><div className="mt-1 flex flex-wrap items-center gap-2"><code className="text-lg font-medium tracking-tight">{proxyAddress||'等待服务器连接'}</code><Button size="sm" variant="ghost" aria-label="复制代理地址" disabled={!proxyAddress} onClick={()=>void copy()}>{copied?<Check/>:<Copy/>}{copied?'已复制':'复制'}</Button></div><p className="mt-1 text-xs text-muted-foreground">在插件中填写此地址，管理网页端口不能用于代理</p></div>
    </CardContent></Card>
    {info&&<SetupSteps hasSubscription={!info.needs_subscription}/>}
    <div className="content-layout"><Nodes snapshot={snapshot} disabled={disabled||!info?.ready||checks.working} busy={busy} select={select} test={()=>checks.start('latency')} sites={()=>setChecksOpen(true)} job={checks.job} latencies={checks.latencies} cancel={checks.cancel}/><aside className="space-y-5">
      <Subscription info={info} disabled={disabled} busy={busy} open={subscriptionOpen} setOpen={setSubscriptionOpen} save={save} update={update}/>
      <Routing routing={routing} disabled={disabled||!!info?.updating||!!info?.starting} changed={refresh}/>
      <p className="px-1 text-xs leading-6 text-muted-foreground"><Network className="mr-1 inline size-3.5"/>Proxy Switcher → fly → 你自己的订阅节点。fly 不自动接管浏览器流量。</p>
    </aside></div>
    <footer className="page-footer"><span>管理网页 {info?`http://${info.admin_addr}`:'连接后显示'}</span><span>仅本机访问 · 命令行启停</span></footer>
    <Diagnostics open={checksOpen} setOpen={setChecksOpen} controls={checks} disabled={disabled||!info?.ready||!!info?.updating}/>
    <SetupHelp open={helpOpen} setOpen={setHelpOpen} port={info?.proxy_port||7888}/>
  </main>;
}
