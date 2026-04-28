# SpringLink

游戏隧道工具箱 - 最初设计为wstunnelGUI，后来加上了frpc（frps服务器自备，ws是被认为是网页服务，我不保证樱花能用）方便管理。为解决UDP游戏联机（僵尸毁灭工程）用网高峰期被SB移动QoS的轻量化wstunnel与frpc的GUI管理工具。

建议使用 [frpc 0.60+](https://github.com/fatedier/frp/)（此前版本未测试）。
建议使用 [wstunnel 0.10.5+](https://github.com/erebe/wstunnel)（此前版本未测试）。

此项目使用Opencode编辑，GLM5.1指导deepseekv4 flash生成，全程0手工代码，所以作者自己也看不懂。包括不会写md文件也是生成+修改。Issue有空就改，不会就摆。

图标由豆包seedance4.5生成+微调+PS。

![Windows](https://img.shields.io/badge/Windows-0078D6?style=flat&logo=windows&logoColor=white)
![Go](https://img.shields.io/badge/Go-00ADD8?style=flat&logo=go&logoColor=white)
![Wails](https://img.shields.io/badge/Wails-v2-2FA4CF?style=flat&logo=wails&logoColor=white)

---

## 特性

- **抗 QoS** — TCP/UDP 流量通过 wstunnel 封装为 WebSocket，规避运营商限速
- **抗 NAT** — 无公网 IP 时通过 frp 中继穿透
- **零命令知识** — 下拉选择 → 一键启动，所有进程命令自动生成
- **极致轻量** — 单一可执行文件，绿色免安装
- **连接码分享** — 导出/导入连接码（`slink://` 协议），快速分享隧道配置
- **系统托盘** — 最小化到托盘，后台运行

---

## 快速开始

### 前置依赖

1. 下载 [wstunnel](https://github.com/erebe/wstunnel) 可执行文件，命名为 `wstunnel.exe`
2. 下载 [frpc](https://github.com/fatedier/frp) 可执行文件，命名为 `frpc.exe`
3. 将两个文件放入 根目录下 `bin/` 目录

### 运行

自行构建（`wails build`），生成的二进制文件位于 `build/bin/` 目录。
或下载 Release 的二进制文件（本项目不包含 frpc 和 wstunnel 文件，请自行下载）。

---

## 使用说明

### 我是服主（服务端）

1. 首次启动，点击「公网配置」，可自动检测公网 IP
2. 添加游戏服务，配置本地端口、传输路径、连接方式（支持端口自动发现）
3. 点击「全部启动」或单独启动某个服务
4. 导出连接码分享给玩家

### 我是玩家（客户端）

1. 添加游戏服务（或导入连接码）
2. 点击「全部连接」或单独连接某个服务
3. 连接成功后，连接地址显示本地转发地址，点击复制连接即可

### 传输组合

| 传输路径     | 连接方式     | 场景                                                |
|----------|----------|---------------------------------------------------|
| 直连       | 无封装（raw） | 有公网 IP，端口直通                                       |
| 直连       | wstunnel | 有公网 IP，WebSocket 封装抗 QoS                          |
| frp 中继   | 无封装（raw） | 无公网 IP，有公网服务器，frp穿透                               |
| frp 中继   | wstunnel | 无公网 IP，有公网服务器，wstunnel封装抗QoS，frp穿透外网传输            |
| wstunnel | 无封装（raw） | 无公网 IP，有公网服务器，自身网络状态差，仅引入自身封装延迟                   |
| wstunnel | wstunnel | 无公网 IP，有公网服务器，只使用wstunnel，双方网络环境差，类似于frp+wstunnel |

---

## 技术栈

- **后端**: Go 1.23 + Wails v2
- **前端**: Svelte 3 + Vite 3
- **进程管理**: os/exec + taskkill（Windows）
- **配置格式**: TOML
- **公网 IP 检测**: STUN（默认 `stun.l.google.com:19302`）

---

## 项目结构

```
game-tunnel/
├── main.go               # Wails 入口
├── app.go                # App 结构体 + Bridge 方法
├── wails.json            # Wails 项目配置
├── configs/              # 默认配置模板
│   └── default.toml
├── internal/
│   ├── codec/            # 连接码编解码（JSON → zlib → xor → base64 URL-safe）
│   ├── config/           # TOML 配置读写与版本迁移
│   ├── network/          # 公网 IP 检测（STUN）
│   ├── orchestrator/     # 服务编排（构建 frpc/wstunnel 命令链）
│   ├── port/             # 端口分配（10000-20000）与游戏端口发现
│   ├── process/          # 进程生命周期管理
│   └── tray/             # 系统托盘（Windows Shell_NotifyIcon）
└── frontend/             # Svelte 前端
    └── src/
        ├── App.svelte
        ├── TitleBar.svelte     # 自定义标题栏（frameless 窗口拖拽、主题切换、托盘）
        ├── ServerTab.svelte    # 服务端标签页
        ├── ClientTab.svelte    # 客户端标签页
        ├── LogPanel.svelte     # 共享日志面板组件
        ├── serverStore.js      # 服务端持久化状态
        ├── clientStore.js      # 客户端持久化状态（进程 ID、连接地址）
        └── storeUtils.js       # 存储工具函数
```

---

## 连接码协议（`slink://`）

包含字段：版本、名称、协议、封装方式、传输路径、本地（游戏）端口、远程地址、远程端口、隧道端口、TLS 等。

注意：此为简单混淆，**非加密安全**，请勿用于敏感场景。

---

## 架构决策

- **无全局 frp 设置**：所有 frp 地址、端口、令牌均为每服务独立配置，通过每行 ⚙️ 模态框设置
- **统一 wstunnel 客户端**：客户端不区分直连/frp 传输，统一由 `BuildClientCommands` 处理
- **自动保存模态框**：所有设置模态框仅含「关闭」按钮，字段通过 `on:change` / `on:input` 自动保存
- **进程隐藏**：Windows 下通过 `HideWindow` 隐藏 wstunnel/frpc 控制台窗口
- **轻量级**：不依赖 frp 健康检查，异常退出时级联停止

---

## 致谢

所有为开源社区贡献一份力的同志。
