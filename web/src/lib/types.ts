export const GROUP = '🚀 Proxy';
export const AUTO = '♻️ 自动选择';
export interface Info {
  version: string; proxy_port: number; admin_addr: string; nodes: number;
  needs_subscription: boolean; ready: boolean; starting: boolean; updating: boolean;
  current?: string; auto_now?: string; update_err?: string; last_err?: string;
  subscription?: { used: number; total: number; expire: number; fetched_at: number };
}
export interface Proxy { now?: string; all?: string[]; history?: { delay:number }[] }
export type ProxyMap = Record<string, Proxy>;
export interface RuleData {
  rules: {type:string;payload:string;target:string}[];
  providers: {name:string;count:number}[];
}
export interface Snapshot { info:Info; proxies:ProxyMap; routing:RuleData }
export const label = (name:string) => name===AUTO?'自动选择':name===GROUP?'所选节点':name==='DIRECT'?'直连':name;
