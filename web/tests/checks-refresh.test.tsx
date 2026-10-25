import {renderHook,act,waitFor} from '@testing-library/react';
import {afterEach,expect,it,vi} from 'vitest';
import {useChecks} from '@/lib/use-checks';
afterEach(()=>vi.unstubAllGlobals());
it('never displays previous-subscription latency after a generation change even while the new poll is pending',async()=>{
 let calls=0;
 vi.stubGlobal('fetch',vi.fn(async(_url:string,options?:RequestInit)=>{
  calls++;
  if(calls>1)return new Promise<Response>((_,reject)=>options?.signal?.addEventListener('abort',()=>reject(new DOMException('Aborted','AbortError'))));
  return Response.json({generation:1,job:null,targets:[],latencies:{A:{name:'A',url:'',status:'ok',delay:42,at:'2026-10-25T00:00:00Z'}}});
 }));
 const {result,rerender}=renderHook(({generation})=>useChecks(generation),{initialProps:{generation:1}});
 await waitFor(()=>expect(result.current.latencies.A.delay).toBe(42));
 await act(async()=>rerender({generation:2}));
 expect(result.current.latencies).toEqual({});
});
