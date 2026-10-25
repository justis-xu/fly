'use client';
import { useCallback, useEffect, useRef, useState } from 'react';
import { api, jsonBody, message } from './api';
import { type Info, type ProxyMap, type RuleData, type Snapshot } from './types';

export function useDashboard() {
  const [snapshot,setSnapshot]=useState<Snapshot|null>(null);
  const [connectionError,setConnectionError]=useState('');
  const [operation,setOperation]=useState<{text:string;error:boolean}|null>(null);
  const [busy,setBusy]=useState<'select'|'test'|'update'|'save'|null>(null);
  const alive=useRef(false);
  const loading=useRef(false);
  const timer=useRef<ReturnType<typeof setTimeout>|null>(null);
  const controller=useRef<AbortController|null>(null);
  const busyRef=useRef(false);
  const refreshRef=useRef<()=>Promise<void>>(async()=>{});
  const refresh=useCallback(async()=>{
    if(loading.current&&!controller.current?.signal.aborted)return;
    if(timer.current)clearTimeout(timer.current);
    loading.current=true;
    const request=new AbortController();controller.current=request;
    let delay=30000;
    try {
      const info=await api<Info>('info',{signal:request.signal});
      const [proxies,routing]=await Promise.all([
        api<{proxies:ProxyMap}>('proxies',{signal:request.signal}),
        api<RuleData>('rules',{signal:request.signal}),
      ]);
      if(alive.current&&!request.signal.aborted) {setSnapshot({info,proxies:proxies.proxies||{},routing});setConnectionError('');}
      if(info.starting||info.updating)delay=2000;
    } catch(error) {
      if(!request.signal.aborted&&alive.current) {
        setConnectionError('无法连接 fly，请在终端运行 fly start 后重试。');delay=5000;
      }
    } finally {
      if(controller.current===request)loading.current=false;
      if(alive.current&&!request.signal.aborted)timer.current=setTimeout(()=>void refreshRef.current(),delay);
    }
  },[]);
  useEffect(()=>{
    alive.current=true;refreshRef.current=refresh;void refresh();
    return()=>{alive.current=false;if(timer.current)clearTimeout(timer.current);controller.current?.abort();};
  },[refresh]);
  const act=async(kind:NonNullable<typeof busy>,path:string,options:RequestInit,prefix:string)=>{
    if(busyRef.current)return false;
    busyRef.current=true;setBusy(kind);setOperation(null);
    try {
      const result=await api<Record<string,number>>(path,options);
      if(kind==='test') {
        setSnapshot(previous=>{
          if(!previous)return previous;
          const proxies={...previous.proxies};
          for(const [name,delay]of Object.entries(result))if(proxies[name])proxies[name]={...proxies[name],history:[{delay}]};
          return {...previous,proxies};
        });
        setOperation({text:`测速完成，${Object.values(result).filter(n=>n>0).length} 个节点可用。`,error:false});
      } else {
        // A slow background poll must not hide the result of a user action.
        controller.current?.abort();
        await refresh();
      }
      return true;
    } catch(error) {setOperation({text:`${prefix}：${message(error)}`,error:true});return false;}
    finally {busyRef.current=false;setBusy(null);}
  };
  return {snapshot,connectionError,operation,busy,dismiss:()=>setOperation(null),
    select:(name:string)=>act('select','select',{method:'PUT',...jsonBody({name})},'切换失败'),
    test:()=>act('test','test',{method:'POST'},'测速失败'),
    update:()=>act('update','update',{method:'POST'},'更新失败'),
    save:(url:string)=>act('save','subscription',{method:'POST',...jsonBody({url})},'保存失败'),
  };
}
