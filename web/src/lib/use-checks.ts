'use client';
import {useEffect,useState,useRef} from 'react';
import {api,jsonBody,message} from './api';
import type {CheckData,CheckJob} from './types';
export function useChecks(generation?:number) {
 const action=useRef(false);const [pollError,setPollError]=useState('');
 const [data,setData]=useState<CheckData|null>(null);const [error,setError]=useState('');const [working,setWorking]=useState(false);const [revision,setRevision]=useState(0);
 useEffect(()=>{
  const controller=new AbortController();let timer:ReturnType<typeof setTimeout>;
  const poll=async()=>{
   let delay=10000;
   try{const d=await api<CheckData>('checks',{signal:controller.signal});if(!controller.signal.aborted){setData(d);setPollError('');if(d.job&&!d.job.done)delay=1000;}}
   catch{if(!controller.signal.aborted){setPollError('无法读取测试进度，请确认 fly 正在运行。');delay=5000;}}
   if(!controller.signal.aborted)timer=setTimeout(()=>void poll(),delay);
  };void poll();return()=>{controller.abort();clearTimeout(timer);};
 },[generation,revision]);
 const start=async(mode:CheckJob['mode'])=>{
  if(action.current)return false;action.current=true;setWorking(true);setError('');
  try{const result=await api<{job:CheckJob}>('checks',{method:'POST',...jsonBody({mode})});setData(previous=>({...previous,job:result.job} as CheckData));setRevision(v=>v+1);return true;}
  catch(e){setError(message(e));return false;}finally{action.current=false;setWorking(false);}
 };
 const cancel=async()=>{
  if(action.current)return;action.current=true;setWorking(true);setError('');
  try{await api('checks?id='+encodeURIComponent(data?.job?.id||''),{method:'DELETE'});setRevision(v=>v+1);}
  catch(e){setError(message(e));}finally{action.current=false;setWorking(false);}
 };
 const job=data?.job?{...data.job,stale:data.job.stale||(generation!==undefined&&data.job.generation!==generation)}:null;
 return {job,targets:data?.targets||[],latencies:!data||generation!==undefined&&data.generation!==generation?{}:data.latencies||{},working,error:error||pollError,start,cancel};
}
