import { execFileSync } from 'node:child_process';
import { readdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { normalizeHTML } from './normalize-html.mjs';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const prefix='internal/server/ui/';
const committed=execFileSync('git',['ls-tree','-r','--name-only','HEAD','--',prefix],{cwd:root,encoding:'utf8'}).trim().split('\n').filter(Boolean).sort();
const actual=[];
function visit(relative) {
  for(const item of readdirSync(path.join(root,relative),{withFileTypes:true})) {
    const file=path.posix.join(relative,item.name);
    if(item.isDirectory())visit(file);else actual.push(file);
  }
}
visit(prefix);
if(JSON.stringify(committed)!==JSON.stringify(actual.sort()))throw new Error('Embedded UI file list is stale; run make frontend and commit the output');
const changed=[];
for(const file of actual) {
  const saved=execFileSync('git',['show',`HEAD:${file}`],{cwd:root,maxBuffer:10*1024*1024});
  const built=readFileSync(path.join(root,file));
  const same=file.endsWith('.html')?normalizeHTML(saved.toString())===normalizeHTML(built.toString()):saved.equals(built);
  if(!same)changed.push(file);
}
if(changed.length)throw new Error(`Embedded UI is stale; run make frontend and commit:\n${changed.join('\n')}`);
console.log(`Verified ${actual.length} shipped files; HTML Flight stream boundaries normalized for comparison only`);
