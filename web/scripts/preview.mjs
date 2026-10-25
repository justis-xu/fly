// Isolated manual QA + documentation preview. Never reads the user's ~/.fly data.
import http from 'node:http';
import {readFile} from 'node:fs/promises';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../../internal/server/ui');
const args=process.argv.slice(2);
const port=Number(args[0]||7899);
const fresh=args.includes('--fresh');
let subscribed=!fresh;
let selected='♻️ 自动选择';
const names=['香港 01','香港 02','日本 01','日本 02','新加坡 01','新加坡 02','美国 01','德国 01'];
const proxies=()=>subscribed?{
 '🚀 Proxy':{now:selected,all:['♻️ 自动选择','DIRECT',...names]},
 '♻️ 自动选择':{now:'香港 01'},
 ...Object.fromEntries(names.map((name,index)=>[name,{history:[{delay:38+index*39}]}])),
}:{};
const server=http.createServer(async(req,res)=>{
 const url=new URL(req.url,'http://127.0.0.1');
 const json=(value,status=200)=>{res.writeHead(status,{'Content-Type':'application/json','Cache-Control':'no-store'});res.end(JSON.stringify(value));};
 if(url.pathname.startsWith('/api/')){
  const chunks=[];for await(const chunk of req)chunks.push(chunk);
  let body={};try{body=JSON.parse(Buffer.concat(chunks).toString()||'{}');}catch{return json({error:'bad request'},400);}
  switch(url.pathname){
   case '/api/info':return json({version:'0.1.0',proxy_port:7888,admin_addr:'127.0.0.1:7889',ready:subscribed,starting:false,updating:false,needs_subscription:!subscribed,nodes:subscribed?8:0,current:selected,auto_now:'香港 01',subscription:subscribed?{used:12.6*1073741824,total:100*1073741824,expire:1893456000,fetched_at:1792886400}:undefined});
   case '/api/proxies':return json({proxies:proxies()});
   case '/api/rules':return json({rules:subscribed?[{type:'RuleSet',payload:'国外网站',target:'🚀 Proxy'},{type:'RuleSet',payload:'国内网站',target:'DIRECT'},{type:'Match',payload:'',target:'🚀 Proxy'}]:[],providers:subscribed?[{name:'国外网站',count:2456},{name:'国内网站',count:18320}]:[]});
   case '/api/subscription':if(req.method==='POST'){subscribed=true;return json({ok:true});}return json({url:subscribed?'https://example.com/subscription/demo-only':''});
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
