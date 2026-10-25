import {render,screen} from '@testing-library/react';
import {expect,it,vi} from 'vitest';
import {Diagnostics} from '@/components/diagnostics';
it('keeps restricted website responses separate from failed connections with the pinned node visible',()=>{
 render(<Diagnostics open setOpen={vi.fn()} disabled={false} controls={{targets:[],latencies:{},working:false,error:'',start:async()=>true,cancel:async()=>{},job:{id:'1',mode:'sites',route:'node',node:'Pinned node',generation:1,started_at:'2026-10-25T10:00:00Z',done:true,stale:false,completed:2,results:[{name:'ChatGPT',url:'https://chatgpt.com/',status:'restricted',http_status:403,at:'2026-10-25T10:00:01Z'},{name:'GitHub',url:'https://github.com/robots.txt',status:'timeout',at:'2026-10-25T10:00:02Z'}]}}}/>);
 expect(screen.getByText('Pinned node')).toBeVisible();
 expect(screen.getByText('访问受限 · HTTP 403')).toBeVisible();
 expect(screen.getByText('连接超时')).toBeVisible();
});
