import { AUTO,type ProxyMap,type CheckResult } from './types';
export type SortOrder='subscription'|'name'|'latency'|'available';
export interface Metric {status:'ok'|'failed'|'timeout'|'untested';delay?:number;at?:string;source:'manual'|'auto';message?:string}
export function nodeMetric(name:string,proxies:ProxyMap,latencies:Record<string,CheckResult>):Metric {
 const history=proxies[name]?.history||[];const last=history.at(-1);const manual=latencies[name];
 if(manual?.at&&['ok','failed','timeout'].includes(manual.status)&&(!last?.time||Date.parse(manual.at)>=Date.parse(last.time))) {
  return {status:manual.status as Metric['status'],delay:manual.delay,at:manual.at,source:'manual',message:manual.message};
 }
 return last?{status:last.delay>0?'ok':'failed',delay:last.delay||undefined,at:last.time,source:'auto'}:{status:'untested',source:'auto'};
}
export function sortNodes(names:string[],order:SortOrder,proxies:ProxyMap,latencies:Record<string,CheckResult>):string[]{
 const rank=(name:string)=>{const m=nodeMetric(name,proxies,latencies);return m.status==='ok'?0:m.status==='untested'?1:2;};
 return [...names].sort((a,b)=>{
  if(a===AUTO)return -1;if(b===AUTO)return 1;
  if(order==='name')return a.localeCompare(b,'zh-CN',{numeric:true});
  if(order==='available'||order==='latency') {const ranked=rank(a)-rank(b);if(ranked)return ranked;if(order==='latency'&&rank(a)===0)return (nodeMetric(a,proxies,latencies).delay||0)-(nodeMetric(b,proxies,latencies).delay||0);}
  return 0;
 });
}
