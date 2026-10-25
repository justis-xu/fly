import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import Page from '@/app/page';
import { StrictMode } from 'react';

let fresh = false;
let failSelect = false;
let failUpdate = false;
let failInfo = false;
let failSubscription = false;
const info = () => ({ version:'0.1.0', proxy_port:7888, admin_addr:'127.0.0.1:7889', nodes:fresh?0:2, needs_subscription:fresh, ready:!fresh, starting:false, updating:false, current:'香港 A', subscription:{used:1073741824,total:10737418240,expire:1893456000,fetched_at:1792886400} });
beforeEach(() => {
  fresh=false;failSelect=false;failUpdate=false;failInfo=false;failSubscription=false;
  vi.stubGlobal('fetch', vi.fn(async (url:string, options?:RequestInit) => {
    await Promise.resolve();
    if(options?.signal?.aborted)throw new DOMException('Aborted','AbortError');
    const path=String(url);
    if(path==='/api/info'&&failInfo) throw new Error('offline');
    if(path==='/api/select'&&failSelect)return new Response('测试节点连接失败',{status:500});
    if(path==='/api/update'&&failUpdate)return new Response('测试订阅更新失败',{status:500});
    if(path==='/api/subscription'&&failSubscription)return new Response('读取订阅失败',{status:500});
    let data:unknown={ok:true};
    if(path==='/api/info')data=info();
    else if(path==='/api/proxies')data={proxies:fresh?{}:{'🚀 Proxy':{now:'香港 A',all:['♻️ 自动选择','DIRECT','香港 A','香港 B']},'♻️ 自动选择':{now:'香港 A'},'香港 A':{history:[{delay:125}]},'香港 B':{history:[]}}};
    else if(path==='/api/rules')data={rules:[{type:'Match',payload:'',target:'🚀 Proxy'}],providers:[{name:'示例规则',count:10}]};
    else if(path==='/api/subscription'&&!options?.method)data={url:fresh?'':'https://example.org/private-token'};
    else if(path==='/api/test')data={'香港 A':120,'香港 B':0};
    return Response.json(data);
  }));
});
afterEach(()=>{vi.useRealTimers();vi.unstubAllGlobals();});

it('guides a fresh user to add their own masked subscription',async()=>{
  fresh=true;const user=userEvent.setup();render(<Page/>);
  await user.click(await screen.findByRole('button',{name:'添加订阅'}));
  expect(screen.getByRole('dialog')).toBeInTheDocument();
  const field=screen.getByLabelText('订阅地址');
  expect(field).toHaveAttribute('type','password');
  await user.type(field,'ftp://example.org/sub');
  await user.click(screen.getByRole('button',{name:'保存并更新'}));
  expect(await screen.findByText('请输入有效的 http 或 https 订阅地址。')).toBeVisible();
});
it('searches nodes without showing DIRECT as a node choice',async()=>{
  const user=userEvent.setup();render(<Page/>);
  await screen.findByRole('button',{name:'选择节点：香港 A'});
  await user.type(screen.getByLabelText('搜索节点'),'香港 B');
  expect(screen.queryByRole('button',{name:'选择节点：香港 A'})).not.toBeInTheDocument();
  expect(screen.getByRole('button',{name:'选择节点：香港 B'})).toBeVisible();
  expect(screen.queryByRole('button',{name:'选择节点：DIRECT'})).not.toBeInTheDocument();
});
it('retains a failed selection message through background refresh',async()=>{
  failSelect=true;const user=userEvent.setup();render(<Page/>);
  await user.click(await screen.findByRole('button',{name:'选择节点：香港 B'}));
  expect(await screen.findByText('切换失败：测试节点连接失败')).toBeVisible();
  vi.useFakeTimers();await vi.advanceTimersByTimeAsync(31000);vi.useRealTimers();
  expect(screen.getByText('切换失败：测试节点连接失败')).toBeVisible();
  expect(screen.getByRole('button',{name:'选择节点：香港 A'})).toHaveAttribute('aria-pressed','true');
});
it('shows failed updates and makes retry available',async()=>{
  failUpdate=true;const user=userEvent.setup();render(<Page/>);
  await screen.findByRole('button',{name:'选择节点：香港 A'});
  await user.click(screen.getByRole('button',{name:'更新订阅'}));
  expect(await screen.findByText('更新失败：测试订阅更新失败')).toBeVisible();
  expect(screen.getByRole('button',{name:'更新订阅'})).toBeEnabled();
});
it('copies the proxy address rather than the management address',async()=>{
  const user=userEvent.setup();render(<Page/>);
  await screen.findByRole('button',{name:'选择节点：香港 A'});
  await user.click(screen.getByRole('button',{name:'复制代理地址'}));
  expect(await navigator.clipboard.readText()).toBe('127.0.0.1:7888');
  expect(screen.getByRole('button',{name:'复制代理地址'})).toHaveTextContent('已复制');
});
it('does not expose a subscription token in page text',async()=>{
  const user=userEvent.setup();render(<Page/>);
  await user.click(await screen.findByRole('button',{name:'修改订阅地址'}));
  await waitFor(()=>expect(screen.getByLabelText('订阅地址')).toHaveValue('https://example.org/private-token'));
  expect(screen.getByLabelText('订阅地址')).toHaveAttribute('type','password');
  expect(document.body.textContent).not.toContain('private-token');
});
it('reports a disconnected server and keeps setup help available',async()=>{
  failInfo=true;render(<Page/>);
  expect(await screen.findByText('无法连接 fly，请在终端运行 fly start 后重试。')).toBeVisible();
  expect(screen.getByRole('button',{name:'使用帮助'})).toBeEnabled();
});
it('never offers to overwrite an unreadable existing subscription',async()=>{
  failSubscription=true;const user=userEvent.setup();render(<Page/>);
  await user.click(await screen.findByRole('button',{name:'修改订阅地址'}));
  expect(await screen.findByText('读取订阅失败，请关闭后重试。')).toBeVisible();
  expect(screen.getByRole('button',{name:'保存并更新'})).toBeDisabled();
});

it('loads correctly when React remounts effects in StrictMode',async()=>{
  render(<StrictMode><Page/></StrictMode>);
  expect(await screen.findByRole('button',{name:'选择节点：香港 A'})).toBeVisible();
});
