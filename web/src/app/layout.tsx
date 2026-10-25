import type { Metadata } from 'next';
import './globals.css';
export const metadata: Metadata = { title: 'fly · 本地代理服务器', description: '配合 Proxy Switcher 使用的本地代理服务器' };
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="zh-CN"><body>{children}</body></html>;
}
