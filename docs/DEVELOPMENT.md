# 开发与检查

这份文件面向修改代码的人。普通用户请直接看[安装教程](INSTALL.md)，不需要启动 Next.js。

## 架构和端口

```text
web/src（Next.js / React / shadcn/ui）
       ↓ npm run build：静态导出
internal/server/ui（提交进 Git）
       ↓ go:embed
fly：7889 管理页与 API；7888 HTTP / SOCKS5 代理
```

运行程序只有一个 Go 进程，不运行 Next Node 服务，不新增控制端口。前端使用相对 `/api/*` 地址。管理 API 保持同源限制和仅本机监听。新增路由、异步检查和订阅事务接口见下方说明。

- `web/src/app/`：入口、布局、全局样式。
- `web/src/components/`：节点、订阅、帮助和页面组件。
- `web/src/components/ui/`：shadcn/ui 生成并纳入仓库的组件。
- `web/src/lib/`：API 类型、请求和状态刷新。
- `web/tests/`：交互、订阅链接脱敏和后台刷新竞争回归。
- `web/scripts/export.mjs`：将 Next 导出复制到嵌入目录；只清理生成目录。
- `internal/server/`：嵌入文件服务和管理 API。

## 环境

- Go 1.26.2 或更高（以 `go.mod` 为准）。
- Node.js 24 与 npm，前端依赖以 `web/package-lock.json` 为准。
- 项目当前锁定 Next.js 16.4.3、React 19.3.1；Tailwind CSS 4、shadcn/ui 与 Radix 组件。
- 字体采用系统字体，构建不需要下载 Google Fonts。

## 修改并构建前端

在仓库根目录执行：

```sh
make frontend
```

它会进入 web，用 `npm ci` 按锁文件安装，再构建并更新 `internal/server/ui/`。生成文件要和源码一起提交；不要手工编辑生成 JS/CSS。构建使用源码摘要作为 build ID，以减少无关生成变化。

只安装普通程序：

```sh
make install
fly restart
```

它使用仓库中已提交的静态文件，不需要 Node。若你改了前端，却没有先 `make frontend`，安装的仍是旧页面。

## 自动检查

```sh
cd web
npm ci --no-fund --no-audit
npm test
npm run build
npm run typecheck
cd ..
go test -race ./...
go vet ./...
go build -o fly .
git diff --check
```

或者使用 `make check` 做交互、类型、Go 测试和 vet；发布前仍需先 `make frontend`，检查生成资源是否与源码同步。CI 会重建前端，并逐一比较 `internal/server/ui/` 的文件清单和内容，再执行 Go 检查。React 在不同机器上可能把同一份页面数据拆成不同长度的内联脚本块；校验只合并相邻的 Flight 数据块后比较，其他 HTML 和所有 JS/CSS 等资源仍严格比对。实际嵌入文件保持 Next 原始输出。

另外三个构建回归用例验证 Flight 分块差异可被接受，而真实 HTML / 数据改动仍会被识别。

测试覆盖首次订阅、错误状态、节点搜索、地址复制、凭据遮挡、读取失败保护、StrictMode 请求取消，以及操作成功时遇到慢后台刷新。Go 资源测试会从实际 HTML 解析 JS/CSS 路径并验证响应，缺失资源必须返回 404，旧 `/panel/` 跳回唯一页面。

## 用演示数据预览，不读取个人订阅

先完成前端构建，然后在仓库根目录运行：

```sh
node web/scripts/preview.mjs 7899
```

打开 `http://127.0.0.1:7899`。它只使用脚本内的示例节点和订阅，不启动代理，不读取 `~/.fly/`。首次使用的空状态：

```sh
node web/scripts/preview.mjs 7900 --fresh
```

打开 `http://127.0.0.1:7900`。选择、保存和更新只影响演示内存。拍图完成后，在运行脚本的终端按 `Ctrl+C` 关闭；这些是临时测试端口，不属于 fly 的正常运行架构。

需要 Next 热更新的人可以在 web 中运行 `npm run dev`（通常使用 3000），但 API 没有另配开发代理，不能把它当作完整生产页面。完整交互请使用上面的演示服务，或构建后用 fly 的同源页面。不要把开发服务器当成第二套日常入口。

## 文档和截图

- 所有用户手册与图片保存在 `docs/`，使用仓库相对链接，能直接在 GitHub 阅读。
- fly 页面截图使用演示数据，避免公开真实节点、订阅凭据和流量信息。
- 插件教程固定为 ID `onnfghpihccifgojkpnnncpagjcdbjod`；升级插件后重新核对按钮和表单行为，不能混用同名产品截图。
- 截图来源、对应状态和未验证范围见 [images/README.md](images/README.md)。
- 任何测试输出、日志、截图和提交前都要检查订阅 URL；`~/.fly/` 不在仓库里。

[返回首页](../README.md) · [QA](QA.md)

内核并发检查补充：远端发现 v1.19.33 的日志等级和混合监听器关闭状态存在并发读写。仓库现保存同版本源码，通过本地 replace 应用已记录的并发补丁；安装不需额外工具。除既有日志 / 监听器补丁外，本轮为入站前缀、IP4P 标志、历史队列与 selector 增加受控并发回归，修复前均由 race 检测复现。源码来源和完整差异见 [third_party/README.md](../third_party/README.md)。

## 新电脑 / 新目录的真实程序验收

```sh
go build -o fly .
python3 scripts/acceptance/clean_room.py --binary ./fly --install
```

这个脚本只使用 Python 标准库。它在自己创建的临时目录安装程序，给真实 fly 子进程一个空 HOME、独立 FLY_HOME 和标准系统 PATH，去掉环境代理；节点、订阅、规则源和目标网站都由本地受控服务提供。它实际经过 mixed 端口验证 HTTP / SOCKS5，而不只调用内存函数。

覆盖：首次空状态、重复启动、有效订阅、带认证头的规则源、坏网页 / 坏内核规则回滚、CLI 更换失败、备用代理下载、新旧来源区分、拒绝 / 直连规则、无效编辑保护、订阅下载离线后的重启、导入文件离线重启、凭据文件权限、进程拥有的监听端口和停止。可选的 lsof 清单只检查脚本自己启动的 PID；脚本退出时停止自己的实例并清理自己的目录。

它不读取个人 `~/.fly/`，不启动 Clash、不需要外部互联网目标，也不会修改浏览器代理。编译和安装依赖仍由开发环境 / CI 提供；这不是“未联网下载依赖”的证明。CI 在已完成构建后也执行这一验收。

## 运行时状态与 API

- 订阅先下载 / 展开节点来源 / 适配器校验，再准备规则资源和完整内核配置；启用有效候选、原子写来源缓存，最后保存地址。保存失败恢复旧运行配置和相应缓存。每个来源单独保存，旧来源不会被新来源顶替。
- 自定义规则独立存 `rules.yaml`；归一化后放在订阅或内置规则之前。无效内容不写入。IP 网段规则可对实际解析出的目的 IP 匹配；域名检查接口只解释用户规则，不代替 DNS / 复杂订阅匹配。
- HTTP 规则资源跟随订阅更新，内核独立刷新间隔设为 0；下载直连 → 显式备用 → 已固定的当前节点。已有资源失败可保留缓存并报告，初次必需资源失败拒绝候选。
- `GET/POST/DELETE /api/checks` 管理一个有时限、可取消的异步任务；`generation` 防止旧来源结果用于新配置。手动延迟结果单独保留，网站测试不覆盖它。
- 手动延迟使用单次 HEAD、严格 HTTP 204、5 秒 / 节点、8 并发、90 秒 / 批次。网站使用固定公开地址的 GET 响应头、8 秒 / 站点，401/403/429 与连接错误分开。
- 自动健康每 300 秒执行，要求 Google 204，5 秒超时，使用内核的重复请求计时；不自动改变手动选择。
- 浏览器分流测试经过真实本机端口；当前节点网站测试直接固定实际适配器。自动选择只在该测试开始时解析一次，不通过偷偷切换 selector 来测试。
- `core.Runtime` 保护 fly 的内核替换 / 读取；上游并发补丁仍要保留，不能用这个外层锁掩盖内核连接线程自己的读写竞争。
- 进程记录含 PID、启动时间和完整命令，启动还持有目录内的锁，避免重复实例或对复用 PID 的其他进程发停止信号。兼容旧 PID 时核对旧后台命令。

| API | 作用 |
|---|---|
| GET /api/info | 本机 ready、订阅状态、generation、当前节点及独立健康记录 |
| GET/POST /api/subscription | 读取设置；验证 URL / 本地内容后更换，POST 成功才表示接受 |
| POST /api/update | 启动现有来源更新，info 报进度 / 错误 |
| GET /api/proxies, PUT /api/select | 简化节点视图 / 手动选择；history 使用目标状态校验后的记录 |
| GET /api/rules | 有效顶层规则和资源条数 |
| GET/PUT /api/routing | 读取 / 整体保存用户规则；GET host 参数解释用户匹配 |
| GET /api/rule-providers/{name} | 搜索缓存内容，q / offset / limit，limit 最大 200 |
| GET/POST/DELETE /api/checks | 查询、启动 latency/sites/browser、取消；GET 还返回独立延迟记录 |
| POST /api/test | 旧接口兼容，最多等 20 秒，仅返回完成的观察；新 UI 使用 checks |

所有管理 API 仍只在回环地址、合法 Host / Origin 下使用；响应不缓存。错误脱敏覆盖 HTTP 地址和节点分享 URI，不公开令牌或认证头。

私有数据目录可用绝对路径 FLY_HOME 指定，所有关联命令必须使用同一目录。普通用户按默认 ~/.fly 使用即可。守护进程采用 Unix 接口，当前安装 / GUI 实测范围是 macOS；Windows 不属于支持范围。
