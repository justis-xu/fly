# 手册图片来源

这些文件与手册一起提交到 GitHub，Markdown 使用相对路径。fly 界面配色为蓝白，演示数据由 `web/scripts/preview.mjs` 提供，不读取用户 `~/.fly/`。

| 文件 | 来源 / 内容 |
|---|---|
| `00-how-it-works.svg` | 手工绘制的使用关系图，不是截图 |
| `01-first-run.png` | 实际 Next 页面 + 空订阅演示 API |
| `02-add-subscription.png` | 实际添加订阅弹窗，示例链接被遮挡 |
| `03-dashboard.png` | 实际 Next 页面 + 示例节点、流量和日期 |
| `04-setup-help.png` | 实际使用帮助弹窗 |
| `05-node-search.png` | 实际搜索日本节点并选中 |
| `06-rules.png` | 实际展开示例分流项和规则集 |
| `07-mobile.png` | 390px 宽的实际页面布局检查；不是手机插件截图 |
| `08-proxy-switcher-settings.svg` | 代理参数示意，不是插件截图 |
| `09-terminal-steps.svg` | 终端命令示意，不是终端实拍 |
| `10-extension-direct.png` | Chrome 实装 Proxy Switcher and Manager 0.6.8，Direct 状态 |
| `11-extension-fill.png` | 同一真实插件：填写 fly / 127.0.0.1 / 7888，HTTP，保存前 |
| `12-extension-saved.png` | 同一真实插件：绿色 ✓ 保存后，配置 fly 出现在列表 |
| `13-extension-store.png` | 官方 Chrome 商店网页，内置浏览器拍摄；页面提示切换到 Chrome；不在内置浏览器安装 |
| `14-extension-pin.png` | Chrome 普通窗口扩展菜单实拍，空白网页背景；裁切保留菜单与入口，Proxy Switcher 已固定 |

采集日期：2026-10-25。插件扩展 ID：`onnfghpihccifgojkpnnncpagjcdbjod`。插件面板实拍没有替换成自制仿图。测试机已安装插件，未重拍全新安装确认弹窗，也未为了拍图卸载个人插件。手册明确说明这一点。

界面更新后，应同步更新有关图片和手册文字。没有在某个浏览器中验证的流程，不写成该浏览器实测。
