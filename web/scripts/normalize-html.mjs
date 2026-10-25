// React may split the same Flight stream into different script chunks depending
// on worker timing. Merge only adjacent frames, preserving all other HTML/order.
export function normalizeHTML(html) {
  const pattern=/<script>self\.__next_f\.push\((\[1,[\s\S]*?\])\)<\/script>/g;
  let output='',cursor=0,end=-1,stream='';
  const flush=()=>{
    if(end<0)return;
    output+=`<script>self.__next_f.push(${JSON.stringify([1,stream])})</script>`;
    cursor=end;end=-1;stream='';
  };
  for(const match of html.matchAll(pattern)) {
    if(match.index!==end){flush();output+=html.slice(cursor,match.index);}
    const frame=JSON.parse(match[1]);
    if(frame.length!==2||frame[0]!==1||typeof frame[1]!=='string')throw new Error('Unexpected Flight frame');
    stream+=frame[1];end=match.index+match[0].length;
  }
  flush();
  return output+html.slice(cursor);
}
