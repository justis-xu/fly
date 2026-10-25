import { expect,it } from 'vitest';
import { normalizeHTML } from '../scripts/normalize-html.mjs';
const frame=(data:string)=>`<script>self.__next_f.push(${JSON.stringify([1,data])})</script>`;
it('compares identical Flight data independently of stream chunk boundaries',()=>{
  expect(normalizeHTML(`<main>fly</main>${frame('first')}${frame('second')}`)).toBe(normalizeHTML(`<main>fly</main>${frame('firstsecond')}`));
});
it('still detects changes to HTML and Flight data',()=>{
  const original=`<main>fly</main>${frame('first')}`;
  expect(normalizeHTML(original)).not.toBe(normalizeHTML(`<main>changed</main>${frame('first')}`));
  expect(normalizeHTML(original)).not.toBe(normalizeHTML(`<main>fly</main>${frame('changed')}`));
});
it('preserves Flight frame order relative to other scripts and markup',()=>{
  const observer='<script>window.observeFlight()</script>';
  expect(normalizeHTML(frame('A')+observer+frame('B'))).not.toBe(normalizeHTML(frame('A')+frame('B')+observer));
  expect(normalizeHTML(frame('A')+'<div>fly</div>'+frame('B'))).not.toBe(normalizeHTML(frame('A')+frame('B')+'<div>fly</div>'));
});
