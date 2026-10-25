import {render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach,expect,it,vi} from 'vitest';
import {Routing} from '@/components/routing';
import type {UserRule} from '@/lib/types';
afterEach(()=>vi.unstubAllGlobals());
it('edits, reorders and saves first-match rules as one transaction',async()=>{
 let persisted:UserRule[]=[];
 const fetch=vi.fn(async(url:string,options?:RequestInit)=>{
  if(options?.method==='PUT'){persisted=JSON.parse(String(options.body)).user_rules;return Response.json({ok:true});}
  return Response.json({user_rules:persisted});
 });vi.stubGlobal('fetch',fetch);const user=userEvent.setup();
 render(<Routing routing={{rules:[],providers:[]}} disabled={false} changed={async()=>{}}/>);
 await user.click(screen.getByRole('button',{name:'查看 / 修改规则'}));
 await user.click(await screen.findByRole('button',{name:'添加规则'}));
 await user.type(screen.getByLabelText('第 1 条规则目标'),'a.example.org');
 await user.click(screen.getByRole('button',{name:'添加规则'}));
 await user.type(screen.getByLabelText('第 2 条规则目标'),'b.example.org');
 await user.selectOptions(screen.getByLabelText('第 2 条规则动作'),'reject');
 await user.click(screen.getByRole('button',{name:'上移第 2 条规则'}));
 await user.click(screen.getByRole('button',{name:'保存规则'}));
 await waitFor(()=>expect(persisted.map(rule=>[rule.value,rule.action])).toEqual([['b.example.org','reject'],['a.example.org','proxy']]));
 expect(await screen.findByText('规则已保存并生效。')).toBeVisible();
});
