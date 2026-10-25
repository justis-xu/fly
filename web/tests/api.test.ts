import {expect,it} from 'vitest';
import {redact} from '@/lib/api';
it('redacts credentials in subscription and resource errors',()=>{
 expect(redact('Get https://example.org/private-token: EOF')).not.toContain('private-token');
 expect(redact('连接失败')).toBe('连接失败');
});
