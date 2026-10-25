export const GROUP = '🚀 Proxy';
export const AUTO = '♻️ 自动选择';
export interface Info {
  version: string; proxy_port: number; admin_addr: string; nodes: number;
  needs_subscription: boolean; ready: boolean; starting: boolean; updating: boolean;
  generation?:number;imported?:boolean;download_route?:string;skipped?:number;
  node_health?:{node:string;status:'untested'|'ok'|'failed';at?:string;delay?:number};
  current?: string; auto_now?: string; update_err?: string; last_err?: string;
  subscription?: { used: number; total: number; expire: number; fetched_at: number };
}
export interface Proxy { now?: string; all?: string[]; history?: { delay:number;time?:string }[];alive?:boolean }
export type ProxyMap = Record<string, Proxy>;
export interface RuleData {
  rules: {type:string;payload:string;target:string}[];
  providers: {name:string;count:number}[];
}
export interface Snapshot { info:Info; proxies:ProxyMap; routing:RuleData }
export const label = (name:string) => name===AUTO?'自动选择':name===GROUP?'所选节点':name==='DIRECT'?'直连':name;
export interface CheckResult {name:string;url:string;status:'pending'|'running'|'ok'|'restricted'|'site_error'|'failed'|'timeout'|'canceled';delay?:number;http_status?:number;message?:string;at?:string}
export interface CheckJob {id:string;mode:'latency'|'sites'|'browser';node:string;route:string;generation:number;started_at:string;finished_at?:string;done:boolean;stale:boolean;cancel_reason?:string;completed:number;results:CheckResult[]}
export interface CheckData {job:CheckJob|null;targets:{name:string;url:string}[];latency_url:string;generation:number;latencies:Record<string,CheckResult>}
export interface UserRule {match:'domain'|'domain_suffix'|'ip_cidr';value:string;action:'proxy'|'direct'|'reject'}
export interface SubscriptionOptions {download_proxy?:string;content?:string}
