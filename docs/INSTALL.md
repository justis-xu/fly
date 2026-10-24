# 安装、升级与卸载

fly 是搭配浏览器插件 **Proxy Switcher** 使用的本地代理服务器，不提供订阅或节点，也不会自动配置浏览器。以下操作面向 macOS。

## 安装

准备 Git、Make、Go 1.26.2 或更高版本。检查：

```sh
git --version
make --version
go version
```

克隆并安装：

```sh
git clone https://github.com/justis-xu/fly.git
cd fly
make install
command -v fly
fly version
```

默认安装到可写的 `/opt/homebrew/bin`；没有该目录或不可写时，安装到 `$(go env GOPATH)/bin`。以安装输出为准。若提示找不到 fly，把实际安装目录加入 shell 的 PATH 后重新打开终端。

首次构建需要联网获取 Go 依赖。安装过程不注册开机启动任务；`fly start` 会把本次服务器放入后台，退出终端后继续运行，重启电脑后需要再次启动。

## 首次设置

```sh
fly start
fly ui
```

在网页填写自己的订阅地址，等到节点和规则加载完成。空订阅不会使用项目自带账号：fly 不内置任何订阅。加载失败时网页保留，方便修改地址。

接着配置 **Proxy Switcher**：HTTP，主机 `127.0.0.1`，端口 `7888`。详细步骤见[使用手册](USAGE.md)。`7889` 是管理网页端口，不是浏览器代理端口。

## 升级

在没有未保存源码改动的项目目录更新：

```sh
git pull --ff-only
make install
fly restart
fly status
```

现有 `~/.fly/` 配置与缓存会保留。升级后重新打开 [管理网页](http://127.0.0.1:7889)。旧版单独的内核控制地址已取消。

## 卸载

先在 Proxy Switcher 中切回浏览器直连，再执行：

```sh
fly stop
make uninstall
```

未运行时 `fly stop` 会提示；可继续卸载。`make uninstall` 从当前 Makefile 选定的安装目录移除程序，不删除 `~/.fly/`，因此可恢复配置。若你曾自行移动程序，以 `command -v fly` 找到的实际路径为准。保留配置中含有订阅凭据，勿上传到公共仓库。
