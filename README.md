# Video Agent

面向开发者的单人本地视频 Agent。项目目标是让编程 Agent 通过 CLI/API 读取素材证据、生成可编辑时间线、提交局部修改，并调用可验证的媒体任务导出 MP4。

当前实现了 P1：本地多素材导入、SQLite 版本化时间线、确定性编辑、FFmpeg 预览与 MP4 导出。无需调用模型。ASR、视觉理解和完整 Agent 接入属于后续阶段。

## 产品定义

这是一个面向开发者的本地视频 Agent 工具链，不是剪映替代品，也不是通用 Code Agent。Code Agent 负责理解需求和规划步骤，Video Agent 负责通过结构化时间线、版本化编辑和可验证渲染把视频可靠地做出来。完整定位见 [产品定义](docs/product-definition.md)。

## 现在有什么

- 项目与多素材导入：ffprobe 元数据、SHA256 去重、本地素材快照。
- SQLite 保存全部时间线版本：裁剪、替换、删除、插入、重排、锁定、解锁、撤销与恢复。
- 原子版本提交：拒绝旧 revision；重复 operation ID 同载荷重放、不同载荷报错。
- 多源渲染：统一帧率和尺寸，处理横竖屏、旋转、原声、单/双声道及无声素材。
- 低分辨率预览、真实 H.264/AAC MP4、持久化任务及逐片段来源记录。
- 验证分辨率、帧数、时长和完整解码后才发布文件；不覆盖已有导出。
- JSON CLI 操作入口；工具注册表与证据检索接口保留给 P2/P3。

## 运行

需要 Go 1.24+、FFmpeg/ffprobe 6+（含 libx264 和 AAC 编码器）：

```sh
go build -o bin/video-agent ./cmd/video-agent
bin/video-agent --help
go test -race ./...
VIDEO_AGENT_INTEGRATION=1 VIDEO_AGENT_ACCEPTANCE_DIR="$PWD/data/p1-acceptance" go test -count=1 -v ./integration
```

最后一个命令生成真实测试视频，经 CLI 完成 36 秒成片、预览、重排、替换和恢复；输出目录由测试日志显示，包含 MP4、版本历史和 JSON 验收报告。没有 FFmpeg 时会明确失败。普通 `go test` 不执行媒体验收。

剪自己的素材请按 [P1 使用说明](docs/p1-usage.md) 操作；[验收记录](docs/p1-acceptance.md) 说明实际验证范围与限制。原内存 `demo` 已被真实 CLI 替代。

## Docker 与 Windows

Docker 版本把 Go、FFmpeg 和 ffprobe 打进镜像；Windows 10/11 用户只需安装并启动 Docker Desktop 的 Linux containers 模式，不需要单独装 Go 或 FFmpeg。素材、数据库和导出文件会留在仓库的 `workspace/`，不会消失在容器内。

PowerShell 中执行：

```powershell
git clone https://github.com/zylar06/video-agent.git
cd video-agent
.\video-agent.bat --help
```

首次调用会自动构建镜像。把视频放入 `workspace\input\`，再使用容器内路径执行命令：

```powershell
.\video-agent.bat project create --id demo --name "我的项目"
.\video-agent.bat assets import --project demo --path /workspace/input/video-1.mp4
```

也可以在 macOS/Linux 使用相同 Compose 配置：

```sh
docker compose run --rm --build video-agent --data /workspace/data --help
```

完整的 Windows 操作、时间线 JSON 例子、备份和排错见 [Docker 使用说明](docs/docker.md)。当前 P1 是命令行工具，Docker 会启动一次命令并退出；后续 P3 HTTP/API 服务会复用同一镜像。

## 目录

```text
cmd/video-agent/       CLI 入口
internal/domain/       素材、片段、时间线和编辑操作
internal/app/          项目服务与渲染任务入口
internal/store/        SQLite 项目、资产、历史版本和任务
internal/edit/         版本校验、编辑补丁和幂等提交
internal/catalog/      分析证据与语义检索接口
internal/media/        本地文件、内容哈希、导入、ffprobe 和子进程
internal/render/       渲染计划、FFmpeg 编译和产物验证
internal/agent/        工具契约和注册表
docs/                  产品、架构和实施计划
examples/              时间线与编辑操作 JSON 样例
integration/           真实媒体和 CLI 端到端验收
```

## 下一步

下一阶段为 P2：字幕/ASR、代表帧、证据缓存、候选检索和初稿规划。P3 再完善 Agent 工具契约、API 与任务观察；P4 处理字幕/BGM、Web 适配和中断恢复。

## 两人协作开发

任务已按 A（媒体与智能）/ B（平台与交互）拆分，包含依赖、模块边界和验收条件。先看 [协作计划](docs/collaboration/README.md)、[GitHub 任务索引](docs/collaboration/github-index.md) 和 [贡献指南](CONTRIBUTING.md)，再认领 Issue。产品仍面向单人本地使用。

完整范围见 [产品定义](docs/product-definition.md)、[需求](docs/requirements.md)、[架构](docs/architecture.md)、[路线图](docs/roadmap.md)、[调研结论](docs/research.md) 和 [验证说明](docs/verification.md)。
