import {render,screen,waitFor} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach,expect,it,vi} from 'vitest';
import {Subscription} from '@/components/subscription';
afterEach(()=>vi.unstubAllGlobals());
it('imports a file with an explicit download proxy and keeps a failed transaction open',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>Response.json({url:'',download_proxy:''})));
 const save=vi.fn(async()=>false),setOpen=vi.fn();const user=userEvent.setup();
 render(<Subscription disabled={false} busy={null} open setOpen={setOpen} save={save} update={async()=>true}/>);
 await user.click(screen.getByRole('button',{name:'本地文件'}));
 await user.upload(screen.getByLabelText('订阅文件'),new File(['proxies:\n- {name: test, type: http, server: example.org, port: 80}'],'nodes.yaml',{type:'text/yaml'}));
 await user.click(screen.getByText('下载打不开？设置备用下载代理'));
 await user.type(screen.getByLabelText('备用下载代理'),'socks5://127.0.0.1:7890');
 await user.click(screen.getByRole('button',{name:'验证并导入'}));
 await waitFor(()=>expect(save).toHaveBeenCalledWith('',{download_proxy:'socks5://127.0.0.1:7890',content:expect.stringContaining('proxies:')}));
 expect(setOpen).not.toHaveBeenCalledWith(false);
});
