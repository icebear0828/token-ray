# 🗺️ 桌面端演进路线图 (Desktop App Roadmap)

> **核心愿景**：告别极客式的终端命令行，将 Token Ray 升级为**"双击安装、开机自启、托盘常驻"**的现代化跨平台桌面客户端，真正实现面向普通用户的"开箱即用"。

---

## 阶段一：Wails/Tauri 架构融合 (GUI 化)
- [x] **技术栈融合**：引入 [Wails](https://wails.io/) 框架，将现有的 Go 核心侦听服务与 React 前端无缝缝合为原生桌面应用。
- [x] **免终端双击启动**：打包输出原生格式包（macOS `.dmg`，Windows `.exe` 安装程序，Linux `.AppImage`）。 *(Phase 4 完善)*
- [x] **标准桌面窗口**：
  - 应用启动后会**直接弹出主界面**，方便查看实时消耗，不再默认隐藏。
  - 保留"开机自启动"能力（可通过操作系统设置），后台探针无感运行。

## 阶段二：配置与操作的完全可视化 (Zero CLI)
- [x] **GUI 参数配置台**：
  - 在设置页可视化修改各个模型的折算价格（取代目前需要改 `pricing_table.json` 然后重新编译的做法）。
  - 可视化修改设备名称 (Device Alias)。
- [x] **路径可视化管理**：允许用户在界面上点击按钮，选择需要额外监听的日志文件夹（而不仅限于默认的 Claude/Gemini 路径）。

## 阶段三：局域网傻瓜式配对 (LAN Auto-Discovery GUI)
- [x] **图形化雷达配对**：彻底废弃通过 `curl` 脚本加入局域网的做法。
  - 新设备安装 APP 并打开后，通过 UDP 广播自动扫描局域网。
  - 界面弹出提示：*"发现局域网主节点 [MacBook-Pro]，是否一键加入该数据雷达网？"*
  - 点击【加入】，底层自动建立数据同传机制。
- [x] **集群拓扑图**：在 UI 界面上绘制出一个酷炫的"雷达地图"，实时显示当前局域网内有哪些节点在线，以及它们正在燃烧多少 Token。

## 阶段四：跨平台分发与自动更新 (Auto-Updater)
- [x] **内置热更新 (OTA)**：检查到 GitHub Release 有新版本时，在应用内一键下载并静默重启完成更新。*(骨架已就位，需接入 selfupdate 实现二进制替换)*
- [x] **移动端前瞻**：PWA `manifest.json` 已就位，局域网内手机可通过浏览器 "Add to Home Screen" 安装。

## 技术债务与重构建议 (Code Review Findings)
- [x] **动态取消监听 (Unwatch)**：`Watcher.UnwatchDir()` 已实现，Settings 移除目录时实时解除监听，无需重启。
- [x] **前端组件拆分**：`SettingsModal.tsx` 已拆分为 `PricingTab` 和 `SystemConfigTab` 子组件。
