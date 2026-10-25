# 检查清单与验证记录

本页区分真实程序、受控故障、演示界面和真实浏览器插件。普通使用先看[安装](INSTALL.md)、[插件操作](PROXY_SWITCHER.md)和[使用手册](USAGE.md)。

## 用户第一次使用的验收表

| 步骤 | 做什么 | 成功信号 |
|---|---|---|
| 安装 | make install，再 fly version | 能找到命令，输出版本 |
| 启动 | fly start，再 fly ui | 唯一网页，空目录提示添加订阅 |
| 添加来源 | 填自己的链接或导入文件 | 验证成功才关闭弹窗并显示节点 |
| 检查本机 | 看页面状态 | 本机代理已启动，仅代表监听就绪 |
| 插件安装 | 核对官方名称和 ID | Proxy Switcher and Manager |
| 插件设置 | Manual Proxy、fly、HTTP/SSL=127.0.0.1:7888、HTTP 类型 | 绿色 ✓ 保存，重开仍保留 |
| 选节点 | 点节点卡片 | 顶部与选中标记同步 |
| 分开测试 | 测速，再测网站 / 浏览器分流 | 不把 Google 延迟当所有网站保证 |
| 查看规则 | 自定义 / 订阅 / 规则集内容 | 能改、保存、搜索和查看资源 |
| 停止 | 插件 Direct，再 fly stop | 浏览器不依赖已经停止的服务 |
| 重启电脑 | fly start，再在插件选 fly | 复用自己的设置，不需要旧电脑缓存 |

## 可重复自动检查

```sh
cd web
npm ci --no-fund --no-audit
npm test
npm run build
npm run typecheck
cd ..
node web/scripts/verify-export.mjs
go test -race ./...
go vet ./...
go build -o fly .
python3 scripts/acceptance/clean_room.py --binary ./fly --install
git diff --check
```

静态导出校验比较提交文件清单、内容和当前源码摘要。只对相邻 React Flight 数据分块做内容等价比较，其他 HTML 和 JS/CSS 等文件严格比较；不跳过 HTML，嵌入资源保持 Next 原始输出。

### 新 HOME / 新配置的真实二进制验收

`clean_room.py` 使用自己的临时 HOME / FLY_HOME、标准系统 PATH、临时端口，无环境代理、旧配置、Clash 和真实订阅。make install 安装到临时目录；真实守护进程、HTTP / SOCKS5 listener、内核节点、订阅服务和目标服务器共同参与，不能只靠 API 返回 ok 过关。

覆盖的故障与实际行为：

| 输入 / 故障 | 验证结果 |
|---|---|
| 从空目录启动 | 网页可用，代理未就绪，没有借用个人订阅 |
| 重复 start | 复用同一后台进程 |
| 订阅自带 9000 / 9090 / allow-lan / TUN | 不采用，PID 拥有的监听只有设定的两个回环 TCP 端口 |
| 正常节点和认证规则源 | 能加载、查看、搜索；HTTP / SOCKS 实际经受控上游到达目标 |
| 新地址返回登录 HTML | 拒绝，地址和旧转发保留 |
| 新订阅有无效内核规则 | 拒绝并恢复原配置 |
| CLI 换坏链接 | 同样拒绝，不绕过 GUI 的事务保护 |
| 主地址直连不可用 | 明确指定的备用代理成功取得来源 B，报告备用路线 |
| 来源 A 换 B | 不沿用 A 的节点冒充 B |
| 拒绝 / 直连规则 | 真实请求分别被拒绝、绕过上游；非法编辑保留原规则 |
| 订阅服务器离线后重启 | 仅用当前来源节点与规则缓存，显示未更新提示，转发仍可工作 |
| 导入本地文件后离线重启 | 来源仍是本地文件，用户规则保留 |
| 凭据存储 | 配置、用户规则、来源缓存为 0600 |
| stop / restart | 自己的实例可停止 / 重启，PID 记录清理 |

脚本在没有 lsof 的环境会明确说明未做进程 socket 清单；其他转发检查仍执行。它不证明第三方节点或网站永远在线，也不证明无网络就能首次下载编译依赖。

### Go 故障回归

还覆盖环境 / 全局代理不影响下载、HTTP 备用、8 MiB 超限、缓存来源指纹、不同来源缓存分别保留、认证请求头、占位名称误删、重名 / 保留名、无名称链接、关键不支持参数、GLOBAL 策略映射、畸形规则拒绝、磁盘保存失败恢复旧内核与缓存、用户规则优先级 / 持久化、IP 网段匹配实际 DNS 解析结果、规则资源离线保留、YAML/text/MRS 搜索翻页、本机 Host/Origin 限制、端口冲突和静态资源 404。

诊断测试用受控 HTTP 目标验证 204 / 403 / 429 / 500、重定向与超时，网站不污染延迟记录，固定节点不受后来选择变化影响，mixed 测试遵循规则；任务并发上限、取消、未测与失败区别和 generation 失效都有回归。

并发检测曾实际发现上游日志、监听关闭、入站前缀、IP4P、队列空检查和 selector 名称竞争；每项先用固定输入复现 race，再修复。完整差异与来源见 [third_party/README.md](../third_party/README.md)，不依赖修改用户共享 Go 模块缓存。

### React 与实际浏览器界面

React 回归覆盖订阅遮挡 / 校验 / 文件导入 / 失败不关闭、排序和新旧测量时间、规则编辑 / 调序 / 保存、403 与超时区别、取消提示、不把未测当失败、旧 generation 的结果失效、节点切换、复制、离线提示、StrictMode 和慢后台请求竞争。错误脱敏包括节点分享 URI。

通过真实构建后的页面和独立演示 API 操作：首次页面、URL / 文件输入、文件选择读取、来源切换、名称 / 延迟排序及刷新保留、规则保存、资源搜索、测速进度 / 取消、网站响应分类。390px 主页面和网站弹窗的文档宽度都为 390px，无横向溢出；临时 viewport 已恢复。控制台未见 warning / error。

**演示 API 不提供代理，不验证互联网。** 图片中的节点、流量和网站状态均是演示数据；真实协议转发由上面的二进制验收单独覆盖。截图来源见 [images/README.md](images/README.md)。

## 真实 Chrome 插件与公网检查

既有插件实测记录（基线 276dcc9）：Chrome 实装 **0.6.8**，ID `onnfghpihccifgojkpnnncpagjcdbjod`，实际填写 fly / 7888 / HTTP，绿色 ✓ 保存后重开保留；Manual Proxy 下 Chrome 曾成功加载 Google robots.txt，随后恢复 Direct。真实插件图片沿用此次相同版本表单。

没有为了拍图卸载现有个人插件，所以没有重拍全新安装确认弹窗。官方商店入口和安装确认按官方帮助核对。Edge / Safari / 手机插件没有实测，不能把 Chrome 截图当成这些平台的验收。

公网复核可使用管理页「测网站」，开发者也可显式指定本机代理进行协议检查：

```sh
curl --noproxy '' --proxy http://127.0.0.1:7888 --max-time 20 -o /dev/null -w '%{http_code}\n' https://www.gstatic.com/generate_204
curl --noproxy '' --socks5-hostname 127.0.0.1:7888 --max-time 20 -o /dev/null -w '%{http_code}\n' https://www.gstatic.com/generate_204
```

预期为 204；它们仍不能代替插件操作、账号登录或所有网站测试。基线曾出现原固定节点超时，而另一个节点 HTTP / SOCKS 均 204；当时恢复了原选择。这是节点可用性会变化的证据，不是本轮节点的当前承诺。

## 交付记录

2026-10-25 本轮已经观察到：独立目录完整验收通过；新的协议 / 规则 / 缓存 / 故障回归通过；构建后的蓝白单页及窄屏检查通过。最终全套检查、独立审查、部署与 GitHub CI 结果在提交推送前复核，不能用旧基线结果代替。

[返回首页](../README.md) · [使用手册](USAGE.md) · [订阅要求](SUBSCRIPTIONS.md)
