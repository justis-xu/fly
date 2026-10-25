import {expect,it} from 'vitest';
import {nodeMetric,sortNodes} from '@/lib/node-metrics';
import {AUTO} from '@/lib/types';
it('keeps auto first and never ranks failed or missing latency as zero milliseconds',()=>{
 const proxies={'fast':{history:[{delay:30}]},'slow':{history:[{delay:250}]},'failed':{history:[{delay:0}]},'unknown':{history:[]}};
 expect(sortNodes(['failed','slow',AUTO,'unknown','fast'],'latency',proxies,{})).toEqual([AUTO,'fast','slow','unknown','failed']);
 expect(sortNodes(['failed','slow',AUTO,'unknown','fast'],'available',proxies,{})).toEqual([AUTO,'slow','fast','unknown','failed']);
});
it('uses the newer manual or automatic observation, keeping their meanings separate',()=>{
 const proxies={A:{history:[{time:'2026-10-25T10:00:00Z',delay:50}]}};
 expect(nodeMetric('A',proxies,{A:{name:'A',url:'',status:'timeout',at:'2026-10-25T10:01:00Z'}}).status).toBe('timeout');
 expect(nodeMetric('A',proxies,{A:{name:'A',url:'',status:'ok',delay:30,at:'2026-10-25T09:00:00Z'}}).delay).toBe(50);
});
