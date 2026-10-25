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

运行程序只有一个 Go 进程，不运行 Next Node 服务，不新增控制端口。前端使用相对 `/api/*` 地址。保留原有 Go API、同源限制和仅本机监听。

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

内核并发检查补充：远端发现 v1.19.33 的日志等级和混合监听器关闭状态存在并发读写。仓库现保存同版本源码，通过本地 replace 应用两个原子状态补丁；安装不需额外工具。已用固定输入复现日志竞争，并连续 10 轮验证核心并发测试通过。源码来源和完整差异见 [third_party/README.md](../third_party/README.md)。
