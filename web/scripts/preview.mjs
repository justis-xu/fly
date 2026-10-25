// Isolated manual QA + documentation preview. Never reads the user's ~/.fly data.
import http from 'node:http';
import {readFile} from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../internal/server/ui');
const args=process.argv.slice(2);
const port=Number(args[0]||7899);
const fresh=args.includes('--fresh');
let subscribed=!fresh;let generation=1;let imported=false;let sourceURL='https://example.com/subscription/demo-only';let downloadProxy='';let userRules=[];let job=null;let latencies={};let sequence=0;
const targets=[['Google','https://www.gstatic.com/generate_204'],['YouTube','https://www.youtube.com/robots.txt'],['GitHub','https://github.com/robots.txt'],['ChatGPT','https://chatgpt.com/'],['Bing','https://www.bing.com/robots.txt'],['百度','https://www.baidu.com/robots.txt']].map(([name,url])=>({name,url}));
const providerData={'国外网站':['+.google.com','+.youtube.com','+.github.com','+.chatgpt.com','+.openai.com','+.wikipedia.org'],'国内网站':['+.baidu.com','+.qq.com','+.bilibili.com','+.taobao.com']};
const invalidate=()=>{generation++;latencies={};if(job){job.stale=true;if(!job.done){job.done=true;job.cancel_reason='配置已变更，请重新测试';}}};
let selected='♻️ 自动选择';
const names=['香港 01','香港 02','日本 01','日本 02','新加坡 01','新加坡 02','美国 01','德国 01'];
const proxies=()=>subscribed?{
 '🚀 Proxy':{now:selected,all:['♻️ 自动选择','DIRECT',...names]},
 '♻️ 自动选择':{now:'香港 01'},
 ...Object.fromEntries(names.map((name,index)=>[name,{history:index===6?[]:[{delay:index===7?0:38+index*39,time:new Date(Date.now()-60000).toISOString()}]}])),
}:{};
const server=http.createServer(async(req,res)=>{
 const url=new URL(req.url,'http://127.0.0.1');
 const json=(value,status=200)=>{res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(value));};
 if(url.pathname.startsWith('/api/')){
  const chunks=[];for await(const chunk of req)chunks.push(chunk);
  let body={};try{body=JSON.parse(Buffer.concat(chunks).toString()||'{}');}catch{return json({error:'bad request'},400);}
  if(url.pathname.startsWith('/api/rule-providers/')){
   const name=decodeURIComponent(url.pathname.slice('/api/rule-providers/'.length));const items=(providerData[name]||[]).filter(item=>item.toLowerCase().includes((url.searchParams.get('q')||'').toLowerCase()));const offset=Number(url.searchParams.get('offset')||0);return json({items:items.slice(offset,offset+100),total:items.length,offset,more:items.length>offset+100});
  }
  switch(url.pathname){
   case '/api/info':return json({version:'0.1.0',proxy_port:7888,admin_addr:'127.0.0.1:7889',ready:subscribed,starting:false,updating:false,needs_subscription:!subscribed,nodes:subscribed?8:0,generation,imported,download_route:imported?'本地文件':'直连',current:selected,auto_now:'香港 01',subscription:subscribed?{used:12.6*1073741824,total:100*1073741824,expire:1893456000,fetched_at:1792886400}:undefined});
   case '/api/proxies':return json({proxies:proxies()});
   case '/api/rules':return json({rules:subscribed?[...userRules.map(r=>({type:{domain:'Domain',domain_suffix:'DomainSuffix',ip_cidr:'IPCIDR'}[r.match],payload:r.value,target:{direct:'DIRECT',reject:'REJECT',proxy:'🚀 Proxy'}[r.action]})),{type:'RuleSet',payload:'国外网站',target:'🚀 Proxy'},{type:'RuleSet',payload:'国内网站',target:'DIRECT'},{type:'Match',payload:'',target:'🚀 Proxy'}]:[],providers:subscribed?Object.entries(providerData).map(([name,items])=>({name,count:items.length})):[]});
   case '/api/subscription':
    if(req.method==='POST'){if(body.url?.includes('bad'))return json({error:'收到的是网页而非订阅，原配置已保留'},400);subscribed=true;imported=body.content!==undefined;sourceURL=imported?'':body.url;downloadProxy=body.download_proxy||'';invalidate();return json({ok:true});}
    return json({url:subscribed?sourceURL:'',download_proxy:downloadProxy,imported});
   case '/api/routing':
    if(req.method==='PUT'){if(body.user_rules?.some(r=>!r.value||r.value.includes(',')))return json({error:'规则目标无效'},400);userRules=body.user_rules;invalidate();return json({ok:true});}
    return json({user_rules:userRules,matched_index:userRules.findIndex(r=>{let host=url.searchParams.get('host')||'';try{host=new URL(host).hostname;}catch{}return host===r.value||r.match==='domain_suffix'&&host.endsWith('.'+r.value);})});
   case '/api/checks':
    if(req.method==='DELETE'){if(job&&!job.done){job.done=true;job.cancel_reason='用户取消，未完成项目不代表节点失败';job.results=job.results.map(r=>['pending','running'].includes(r.status)?{...r,status:'canceled'}:r);}return json({ok:true});}
    if(req.method==='POST'){
     if(job&&!job.done)return json({error:'已有测试正在进行'},409);
     const id=String(++sequence);const tests=body.mode==='latency'?names.map(name=>({name,url:targets[0].url})):targets;
     job={id,mode:body.mode,route:body.mode==='browser'?'browser':'node',node:body.mode==='browser'?'按当前规则分流（可能直连或自动切换）':selected==='♻️ 自动选择'?'香港 01':selected,generation,started_at:new Date().toISOString(),done:false,stale:false,completed:0,results:tests.map(t=>({...t,status:'pending'}))};
     tests.forEach((test,index)=>setTimeout(()=>{if(job.id!==id||job.done)return;const result={...test,status:body.mode==='latency'&&index===7?'timeout':body.mode!=='latency'&&test.name==='ChatGPT'?'restricted':'ok',http_status:body.mode!=='latency'&&test.name==='ChatGPT'?403:body.mode==='latency'||test.name==='Google'?204:200,delay:38+index*39,at:new Date().toISOString()};job.results[index]=result;job.completed++;if(body.mode==='latency')latencies[test.name]=result;if(job.completed===tests.length){job.done=true;job.finished_at=new Date().toISOString();}},600+index*600));
     return json({job});
    }
    return json({job,generation,latencies,targets,latency_url:targets[0].url});
   case '/api/select':if(!names.includes(body.name)&&body.name!=='♻️ 自动选择')return json({error:'unknown node'},400);selected=body.name;return json({ok:true});
   case '/api/update':return json({ok:true});
   case '/api/test':return json(Object.fromEntries(names.map((name,index)=>[name,38+index*39])));
   default:return json({error:'not found'},404);
  }
 }
 const file=url.pathname==='/'?'index.html':url.pathname.slice(1);
 const resolved=path.resolve(root,file);
 if(!resolved.startsWith(root+path.sep)){res.writeHead(403);return res.end();}
 try{
  const content=await readFile(resolved);
  const types={'.html':'text/html; charset=utf-8','.js':'text/javascript','.css':'text/css','.txt':'text/plain','.svg':'image/svg+xml'};
  res.writeHead(200,{'Content-Type':types[path.extname(resolved)]||'application/octet-stream','Cache-Control':'no-store'});res.end(content);
 }catch{res.writeHead(404);res.end('Not found');}
});
server.listen(port,'127.0.0.1',()=>console.log(`Safe fixture preview http://127.0.0.1:${port} ${fresh?'(fresh install)':'(sample subscription)'}`));
