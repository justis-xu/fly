import { act, renderHook } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { useDashboard } from '@/lib/use-dashboard';
afterEach(()=>{vi.useRealTimers();vi.unstubAllGlobals();});
it('refreshes the selected node immediately when a background request is still pending',async()=>{
  vi.useFakeTimers();let current='A';let infoCalls=0;
  vi.stubGlobal('fetch',vi.fn(async(url:string,options?:RequestInit)=>{
    if(url==='/api/select'){current='B';return Response.json({ok:true});}
    if(url==='/api/info'){
      infoCalls++;
      if(infoCalls===2)return new Promise<Response>((_,reject)=>options?.signal?.addEventListener('abort',()=>reject(new DOMException('Aborted','AbortError'))));
      return Response.json({current,ready:true});
    }
    return Response.json(url==='/api/proxies'?{proxies:{}}:{rules:[],providers:[]});
  }));
  const {result}=renderHook(()=>useDashboard());
  await act(async()=>{});
  expect(result.current.snapshot?.info.current).toBe('A');
  await act(async()=>{await vi.advanceTimersByTimeAsync(30000);});
  await act(async()=>{await result.current.select('B');});
  expect(result.current.snapshot?.info.current).toBe('B');
});
