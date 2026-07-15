# sim7600d

[![构建 main 分支产物](https://github.com/NeverBehave/simcom7600/actions/workflows/build-artifacts.yml/badge.svg)](https://github.com/NeverBehave/simcom7600/actions/workflows/build-artifacts.yml)

[English](README.md) | [简体中文](README.zh-CN.md)

`sim7600d` 可以把一台通过 USB 连接的 SIMCom SIM7600 调制解调器变成小型、
自托管的电话服务。Go 守护进程独占调制解调器的 AT 命令端口，将状态保存到
SQLite，提供带认证的 REST API，并托管用于短信和语音通话的 Web 界面。

仓库还包含原生 Android 客户端，提供相同的调制解调器、短信、通话、呼叫转移、
事件和管理功能。即使应用界面没有打开，其前台事件连接仍能实时显示来电和新短信
通知。

![使用合成演示数据的 sim7600d 仪表盘](docs/assets/web-ui-dashboard-demo.jpg)

_仪表盘使用合成演示数据，其中不包含真实电话号码、短信、SIM 或调制解调器标识符、
凭据或通话记录。_

## 功能

- 发送、接收、浏览和删除短信，支持 GSM-7、UCS-2 和多段短信。
- 拨号、接听、拒接和挂断语音通话。
- 读取和修改运营商的无条件、忙线、无人接听和无法接通呼叫转移规则。
- 在通话中发送 DTMF。
- 通过带认证的 WebSocket 在浏览器中通话。
- 通过 SIM7600 原始 USB 音频串口传输 16 位有符号、单声道、16 kHz PCM。
- 使用 SQLite 记录调制解调器状态、通话、短信和事件。
- 在重启或传输错误后，自动协调数据库状态与调制解调器状态。
- 查看 OpenAPI 文档以及可选的管理员诊断功能。
- 使用原生 Android 应用进行双向通话音频、通知操作、对话式短信和全部 Web 控制操作。

推荐的构建方式会将浏览器界面和 API 嵌入同一个 Go 二进制文件。项目也会生成无
Web 界面的服务器和独立静态 Web 包，便于通过反向代理分别部署。服务器默认监听
`127.0.0.1:8080`，并使用一个共享的 Bearer Token 认证。

## 硬件和音频

已验证的部署使用 USB PID 为 `1e0e:9011` 的 SIM7600G-H：

- AT 命令：调制解调器的 AT 串口，通常是 `/dev/ttyUSB3`
- 通话音频：厂商定义的原始音频串口，通常是 `/dev/ttyUSB4`

SIMCom 的“USB AUDIO”接口不是 USB Audio Class，也不会表现为 ALSA 声卡。它是
一个全双工原始 PCM 字符设备。连接浏览器音频前，守护进程通过
`AT+CPCMFRM=1` 选择文档规定的 16 kHz 模式，配置宽带语音路径，然后使用
`AT+CPCMREG=1` 开始 PCM 传输。

不同产品 ID 和固件的 USB 接口编号可能不同。生产环境中建议使用稳定的
`/dev/serial/by-id/` 路径。

## 构建要求

- Go 1.26.2，或 `go.mod` 中声明的版本
- Node.js 22 和 npm，用于构建 Web 界面
- JDK 17 和 Android SDK 35，用于构建 APK
- 能够访问调制解调器 AT 和原始音频串口的 Linux 主机
- 当前用户属于设备所属用户组，通常是 `dialout`

## 构建和测试

```sh
make test
make ui-verify
make build
make artifacts
```

`make build` 会安装 TypeScript 客户端和 Web 界面依赖，构建生产版界面，将其嵌入
守护进程，并生成 `build/sim7600d`。

`make artifacts` 运行与 GitHub Actions 相同的打包脚本，在 `artifacts/` 下生成
带版本号的文件和 SHA-256 校验清单。

## 构建产物

每次推送到 `main` 都会运行
[`Build main artifacts`](.github/workflows/build-artifacts.yml) 工作流。工作流会
测试三套代码，并将以下四种产物保留 14 天：

| 产物 | 内容 | 用途 |
| --- | --- | --- |
| `sim7600d-server-linux-amd64-<version>.tar.gz` | 使用 `headless` 标签构建的 Linux AMD64 Go 服务器 | 将 API/WebSocket 服务器部署在独立静态 Web 主机之后 |
| `sim7600d-web-<version>.tar.gz` | 生产版 `web/ui/dist` 静态文件 | 与 API 同源托管，并将 `/v1` 和 `/openapi.json` 代理到服务器 |
| `sim7600d-server-web-linux-amd64-<version>.tar.gz` | 内嵌 Web 界面的 Linux AMD64 Go 服务器 | 推荐的单二进制部署方式 |
| `sim7600d-android-<version>.apk` | 可安装的 Android APK | `main` 产物使用调试签名；标签发布使用私有发布密钥签名 |

无 Web 界面的服务器会对浏览器应用路由返回 HTTP 404，但 REST API、事件流和通话
音频 WebSocket 仍然可用。两个服务器压缩包内的可执行文件都叫 `sim7600d`。每个
可下载的工作流产物都包含对应的 `.sha256` 文件；正式发布还会提供合并的
`SHA256SUMS-<version>.txt` 清单。

打包目标默认为 Linux AMD64。直接运行 `scripts/build-artifacts.sh` 时，可以覆盖
`TARGET_GOOS`、`TARGET_GOARCH`、`VERSION` 或 `ARTIFACT_DIR`。

## 发布

推送语义化版本标签会运行 [`Release`](.github/workflows/release.yml) 工作流。该
工作流会重复完整的测试和打包流程，创建包含自动生成说明的 GitHub Release，并
附加全部四种产物和校验清单：

```sh
git tag -a v0.2.0 -m "sim7600d v0.2.0"
git push origin v0.2.0
```

只有最终发布任务会获得仓库范围 GitHub Actions Token 的 `contents: write`
权限；源码检出、测试和打包阶段以及普通 `main` 构建都只使用只读权限。

标签发布需要配置以下 GitHub Actions Secrets。任意签名值缺失时，工作流会拒绝
发布调试签名 APK：

- `ANDROID_KEYSTORE_BASE64`：发布密钥库的 Base64 编码
- `ANDROID_KEYSTORE_PASSWORD`
- `ANDROID_KEY_ALIAS`
- `ANDROID_KEY_PASSWORD`

### Android 应用

Android 项目位于 [`android`](android/README.md)。安装 JDK 17 和 Android API 35
SDK 后运行：

```sh
make android-test
make android-build
```

调试 APK 会生成在 `android/app/build/outputs/apk/debug/app-debug.apk`。

重新生成已提交的 OpenAPI 文档和 TypeScript 客户端：

```sh
make openapi
make sdk
make sdk-verify
```

## 配置

创建 TOML 配置文件：

```toml
[server]
bind = "127.0.0.1:8080"
auth_token_file = "/run/secrets/sim7600d-auth-token"

[modem]
tty = "/dev/serial/by-id/usb-SimTech__Incorporated_SimTech__Incorporated_0123456789ABCDEF-if05-port0"
baud = 115200

[audio]
device = "/dev/serial/by-id/usb-SimTech__Incorporated_SimTech__Incorporated_0123456789ABCDEF-if06-port0"

[storage]
path = "/var/lib/sim7600d/sim7600d.db"

[retention]
events_days = 30
sms_days = 365
calls_days = 365
idem_hours = 24

[admin]
at_passthrough = false
allow_modem_reset = false

[log]
level = "info"
at_trace = false
```

然后启动服务：

```sh
./build/sim7600d --config ./sim7600d.toml
```

如果既没有设置 `SIM7600D_AUTH_TOKEN`，也没有配置 `auth_token_file`，守护进程会
在 SQLite 数据库旁创建权限为 `0600` 的 `auth_token` 文件。自定义 Token 至少需要
32 个字符，并且只能包含字母、数字、`-`、`.`、`_` 和 `~`。

保留期限每小时执行一次；设置为 `0` 表示永久保留该类历史记录。未完成的多段短信
片段无论历史保留设置如何，都会在七天后清理。

在浏览器中打开配置的地址并输入该 Token。无需认证即可访问 `/openapi.json` 中的
OpenAPI 3.1 文档；所有调制解调器操作都需要认证。

## 安全

- 除非前方有可信的反向代理或私有隧道，否则请让服务只监听回环地址。
- 将 Bearer Token 当作密码保管。
- 除非在配置中显式开启，否则 AT 命令直通和调制解调器重置功能处于禁用状态。
- 通话音频 WebSocket 通过子协议值传递凭据，而不是将凭据放进 URL。

## 使用远程调制解调器开发

Makefile 提供了一组辅助命令，通过 SSH 和 `socat` 将远程调制解调器 AT 端口转发
到本地 PTY：

```sh
make bridge-remote
make bridge-tunnel
make bridge-local
make run
```

可按需覆盖 `REMOTE`、`RTTY`、`RPORT` 或 `LPTY`。每个物理调制解调器串口同时
只能由一个进程占用。

## 验证记录

实际部署和硬件测试结果记录在 [`docs/verification`](docs/verification/) 中。已验证
的 16 kHz 回声测试覆盖浏览器、调制解调器、蜂窝网络再返回浏览器的完整音频路径。

实现所参考的厂商文档整理在
[`docs/reference`](docs/reference/simcom/README.md) 下。专有原始手册和生成的全文
提取文件仅保存在本地，不随此仓库分发。
