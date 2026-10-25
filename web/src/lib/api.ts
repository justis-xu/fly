export const redact=(text:string)=>text.replace(/https?:\/\/[^\s"<>]+/g,'[订阅或资源地址]');

export async function api<T>(path:string, options:RequestInit = {}):Promise<T> {
  const response = await fetch(`/api/${path}`, { cache:'no-store', ...options });
  if(!response.ok) {
    // Backend errors can contain a subscription URL. Never echo its credentials.
    const text=redact(await response.text());
    throw new Error(text.trim() || `请求失败（${response.status}）`);
  }
  return response.json() as Promise<T>;
}
export const message=(error:unknown)=>error instanceof Error?error.message:'请求失败，请重试。';
export const jsonBody=(value:unknown)=>({headers:{'Content-Type':'application/json'},body:JSON.stringify(value)});
