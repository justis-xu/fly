import {render,screen} from '@testing-library/react';
import {expect,it} from 'vitest';
import {Nodes} from '@/components/nodes';
import type {Snapshot} from '@/lib/types';
it('explains cancellation without turning untested nodes into failed measurements',()=>{
 const snapshot:Snapshot={info:{version:'test',proxy_port:7888,admin_addr:'127.0.0.1:7889',nodes:1,ready:true,needs_subscription:false,starting:false,updating:false},proxies:{'🚀 Proxy':{now:'♻️ 自动选择',all:['♻️ 自动选择','DIRECT','A']},'♻️ 自动选择':{now:'A'},A:{history:[]}},routing:{rules:[],providers:[]}};
 render(<Nodes snapshot={snapshot} disabled={false} busy={null} select={async()=>true} test={async()=>true} sites={()=>{}} cancel={async()=>{}} latencies={{}} job={{id:'1',mode:'latency',node:'',route:'node',generation:1,started_at:'2026-10-25T00:00:00Z',done:true,stale:false,completed:0,cancel_reason:'用户取消，未完成项目不代表节点失败',results:[{name:'A',url:'',status:'canceled'}]}}/>);
 expect(screen.getByRole('status')).toHaveTextContent('用户取消，未完成项目不代表节点失败');
 expect(screen.getByRole('button',{name:'选择节点：A'})).toHaveTextContent('未测速');
});
