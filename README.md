# fly

**搭配浏览器插件 Proxy Switcher 使用的本地代理服务器。**

fly 不附带代理节点或订阅，不自动接管浏览器流量。你提供订阅，Proxy Switcher 把浏览器请求交给 fly，fly 根据订阅规则和你选择的节点转发请求。

```text
浏览器 → Proxy Switcher → fly（127.0.0.1:7888）→ 订阅节点 / 按规则直连
```

命令行负责启停，一个网页负责选节点、测速和管理订阅。不设置系统代理，不启用 TUN。

## 快速开始（macOS）

需要 Git、Make 和 Go 1.26.2 或更高版本。

```sh
git clone https://github.com/justis-xu/fly.git
cd fly
make install
fly start
fly ui
```

1. 在管理网页填写**你自己的订阅地址**，保存并等待加载完成。
2. 在浏览器安装并打开 **Proxy Switcher**，新增一个代理配置：协议 **HTTP**，主机 **127.0.0.1**，端口 **7888**。需要代理的 HTTP 和 HTTPS 网站都使用此配置；HTTPS 网站通过 HTTP CONNECT 访问。
3. 在 Proxy Switcher 中启用这个配置，然后在 fly 网页选择节点或「自动选择」。

也可以使用 SOCKS5 协议，主机和端口不变；插件提供远程 DNS 选项时一并启用。

| 地址 | 用途 |
|---|---|
| `127.0.0.1:7888` | 填入 **Proxy Switcher** 的代理地址，支持 HTTP / SOCKS5 |
| [http://127.0.0.1:7889](http://127.0.0.1:7889) | fly 唯一管理网页，不能填成代理端口 |

以上为默认地址，全部只监听本机。没有额外控制端口；旧 `/panel/` 地址回到同一网页。首次添加订阅前，只有管理网页可用，代理端口尚未监听。

## 日常命令

| 命令 | 用途 |
|---|---|
| `fly` / `fly start` | 后台启动服务器 |
| `fly stop` | 停止服务器；Proxy Switcher 仍指向它时浏览器请求会失败 |
| `fly restart` | 重启服务器 |
| `fly status` | 查看服务器、代理就绪状态、节点与订阅信息 |
| `fly ui` | 打开管理网页，不代替启动命令 |
| `fly update` | 更新订阅，保持节点选择 |
| `fly run` | 前台运行，Ctrl+C 退出 |
| `fly init --url <地址> --port <端口>` | 修改启动配置，重启后生效 |

网页可点击切换节点、测速、更新订阅或修改订阅地址。订阅的直连、拒绝规则保留，需要代理的策略统一使用所选节点；HTTP / inline 规则集显示实际加载数量。没有规则的链接订阅采用内置分流：国内及局域网直连，其余走节点。仅通过 Proxy Switcher 发给 fly 的请求会经过这些规则。

## 手册与检查

- [安装、升级与卸载](docs/INSTALL.md)
- [Proxy Switcher 配置与日常使用](docs/USAGE.md)
- [常见问题与排查](docs/FAQ.md)
- [QA 检查清单与验证记录](docs/QA.md)

## 数据与边界

配置和缓存位于 `~/.fly/`，重启、更新后保留节点选择。失败更新保留可用配置和缓存；冷启动可回退上次缓存，网页明确提示。订阅默认每 24 小时更新。HTTP 规则资源直接下载；下载失败或内容为空会显示「尚未就绪」，不能据此认为规则完整生效。

**订阅地址和缓存含凭据，不要公开分享。** fly 不提供隐藏进程或规避终端安全检测的功能，也不保证本地服务不被识别。

内嵌 [mihomo](https://github.com/MetaCubeX/mihomo) 内核，内置 GeoIP、GeoSite、ASN 基础数据。ASN 数据来自 [MetaCubeX meta-rules-dat](https://github.com/MetaCubeX/meta-rules-dat/releases/tag/latest)，基础数据随二进制发布，订阅规则资源仍需按需下载。许可为 AGPL-3.0，见 [LICENSE](LICENSE)。
