'use client';
import { Check, Link2, Puzzle, MousePointer2, ArrowRight } from 'lucide-react';
import { Card } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';

export function SetupSteps({hasSubscription}:{hasSubscription:boolean}) {
  const steps=[{icon:hasSubscription?Check:Link2,title:'添加自己的订阅',text:hasSubscription?'订阅已配置':'在订阅卡片中填写链接',done:hasSubscription},{icon:Puzzle,title:'配置 Proxy Switcher',text:'把浏览器请求交给 fly',done:false},{icon:MousePointer2,title:'选择一个节点',text:'点击节点，开始使用',done:false}];
  return <Card className="setup-card"><div className="grid gap-5 md:grid-cols-3">
    {steps.map((step,index)=><div key={step.title} className="flex items-center gap-3"><span className={`step-icon ${step.done?'step-done':''}`}><step.icon className="size-4"/></span><div><p className="text-sm font-medium"><span className="mr-1 text-muted-foreground">{index+1}.</span>{step.title}</p><p className="mt-1 text-xs text-muted-foreground">{step.text}</p></div></div>)}
  </div></Card>;
}
export function SetupHelp({open,setOpen,port}:{open:boolean;setOpen:(open:boolean)=>void;port:number}) {
  return <Dialog open={open} onOpenChange={setOpen}><DialogContent className="sm:max-w-xl"><DialogHeader><DialogTitle>第一次使用？只需三步</DialogTitle><DialogDescription>fly 是本地服务器，浏览器需要通过 Proxy Switcher 使用它。</DialogDescription></DialogHeader>
    <ol className="space-y-5 text-sm leading-relaxed">
      <li><strong>1. 添加订阅</strong><p className="mt-1 text-muted-foreground">在主页面点击「添加订阅」，粘贴自己的订阅链接并点「保存并更新」，也可选择「本地文件」导入。等到节点出现、状态显示「本机代理已启动」。</p></li>
      <li><strong>2. 安装并配置 Proxy Switcher and Manager</strong><p className="mt-1 text-muted-foreground">下方图文教程包含官方安装入口。安装后打开插件，点击 Manual Proxy，Profile Name 填 fly，再填写：</p><div className="my-2 rounded-xl border bg-background p-4"><dl className="subscription-data"><div><dt>Server Type</dt><dd>HTTP</dd></div><div><dt>HTTP / SSL Proxy</dt><dd><code>127.0.0.1</code></dd></div><div><dt>两行的 Port</dt><dd><code>{port}</code></dd></div><div><dt>Direct 地址列表</dt><dd><code>localhost, 127.0.0.1</code></dd></div></dl></div><p className="text-muted-foreground">点击 Profile Name 右侧绿色 ✓ 保存并启用。HTTP 和 HTTPS 网站均用这份配置，无须用户名或密码。</p></li>
      <li><strong>3. 选择节点</strong><p className="mt-1 text-muted-foreground">回到 fly，点击想使用的节点，或选择「自动选择」。顶部会显示当前选择。点「测网站」确认可达性；本机已启动只代表端口就绪。需要代理的网站必须由插件交给 fly。</p></li>
    </ol>
    <p className="rounded-lg bg-blue-50 p-3 text-xs leading-relaxed text-blue-800">代理端口和管理网页端口不同。请把上方端口填进 Proxy Switcher；停止 fly 前，先把插件切回直连。</p>
    <a className="inline-flex items-center gap-1 text-sm font-medium text-primary" href="https://github.com/justis-xu/fly/blob/master/docs/PROXY_SWITCHER.md" target="_blank" rel="noreferrer">打开插件安装与使用图文教程<ArrowRight className="size-4"/></a>
  </DialogContent></Dialog>;
}
