import { createHash } from 'node:crypto';
import { readdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
export function sourceHash() {
  const hash=createHash('sha256');
  const visit=(relative)=>{
    for(const entry of readdirSync(path.join(root,relative),{withFileTypes:true}).sort((a,b)=>a.name.localeCompare(b.name,'en'))) {
      const file=path.posix.join(relative,entry.name);
      if(entry.isDirectory())visit(file);
      else {hash.update(file);hash.update(readFileSync(path.join(root,file)));}
    }
  };
  visit('src');
  for(const file of ['package.json','package-lock.json','next.config.ts','postcss.config.mjs','components.json','scripts/export.mjs','scripts/source-hash.mjs','scripts/normalize-html.mjs','scripts/verify-export.mjs']) {hash.update(file);hash.update(readFileSync(path.join(root,file)));}
  return hash.digest('hex').slice(0,20);
}
