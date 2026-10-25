import { cp, mkdir, readFile, rm, rename } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const web = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const output = path.join(web, 'out');
const target = path.resolve(web, '../internal/server/ui');
const temporary = `${target}.building`;
// Fail before changing the shipped UI if Next did not export real assets.
const html = await readFile(path.join(output, 'index.html'), 'utf8');
if (!html.includes('/_next/')) throw new Error('Next.js static export is missing its assets');
await mkdir(path.dirname(target), { recursive:true });
await rm(temporary, { recursive:true, force:true });
await cp(output, temporary, { recursive:true });
await rm(target, { recursive:true, force:true });
await rename(temporary, target);
console.log('Embedded UI refreshed in internal/server/ui');
