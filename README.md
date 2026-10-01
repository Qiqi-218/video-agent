# Video Agent

面向开发者的单人本地视频 Agent。项目目标是让编程 Agent 通过 CLI/API 读取素材证据、生成可编辑时间线、提交局部修改，并调用可验证的媒体任务导出 MP4。

当前实现了 P1、P2（可演示闭环）与 P3 运行时：本地多素材导入、SQLite 版本化时间线、字幕/ASR 证据、代表帧采样、分析缓存、确定性编辑、FFmpeg 预览/导出，以及受限的异步任务、外部 Agent 工具协议与 loopback HTTP API。

## 项目状态

### 已完成

- P1 确定性视频编辑底座：多素材导入、素材哈希、版本化时间线、裁剪/替换/插入/删除/重排、锁定、撤销与恢复。
- FFmpeg 预览和 MP4 导出：统一帧率与画幅，完成分辨率、帧数、时长、编码和完整解码校验后才发布成片。
- P3 异步任务：任务提交、状态查询、进度、取消、失败状态和任务列表，P1 同步 CLI 保持兼容。
- P3 外部 Agent 工具协议：统一 v1 JSON envelope、证据写入、搜索、proposal、编辑提交、冲突处理和版本恢复。
- P3 本地 HTTP API：loopback 服务、受控 MP4 产物读取、HTTP Range 播放和路径穿越防护。
- Docker/Windows 部署：Dockerfile、Compose 和 `video-agent.bat`，Go/FFmpeg/ffprobe 一起打包。
- 真实素材回放：使用指定 B 站视频完成导入、证据检索、连续 5 次编辑、冲突恢复、异步导出和取消验证。
- P2 分析闭环：支持 SRT/VTT 导入、可选 OpenAI-compatible ASR/视觉 provider、最多 48 张代表帧、SQLite 分析运行缓存，以及带来源时间戳的检索/proposal。
- 自动化验证：Go 单元测试、race、vet、P1 FFmpeg 端到端测试、P3 API 测试和 Docker API 烟测。

### 未完成

- P2 的高级理解：场景切分、embedding/情绪与节奏评分，以及针对复杂创作目标的质量验收。
- P2 的模型 provider 需要用户配置 API；没有 provider 时仍可用本地字幕和代表帧完成可追溯闭环。
- P4 产品交互：浏览器 Web 界面、对话式编辑、时间线预览、局部修改交互。
- P4 成片能力：中文字幕渲染、BGM/混音、9:16 fit/crop、复杂构图和更细的风格控制。
- P4 可靠性：服务重启后的任务恢复、自动重试、临时文件回收和工程目录迁移后的路径修复。
- MCP 与内置自主 Agent 循环：目前不是必选范围，外部 Code Agent 通过 CLI/HTTP 调用即可。

当前边界：这是一个具备 P1/P2/P3 工程闭环的视频 Agent 执行层，不是剪映替代品，也不是已经完成的通用对话式视频产品。P4 才覆盖完整的“对话规划 → 可视化编辑 → 成片”体验。

## 产品定义

这是一个面向开发者的本地视频 Agent 工具链，不是剪映替代品，也不是通用 Code Agent。Code Agent 负责理解需求和规划步骤，Video Agent 负责通过结构化时间线、版本化编辑和可验证渲染把视频可靠地做出来。完整定位见 [产品定义](docs/product-definition.md)。

## 现在有什么

- 项目与多素材导入：ffprobe 元数据、SHA256 去重、本地素材快照。
- SQLite 保存全部时间线版本：裁剪、替换、删除、插入、重排、锁定、解锁、撤销与恢复。
- 原子版本提交：拒绝旧 revision；重复 operation ID 同载荷重放、不同载荷报错。
- 多源渲染：统一帧率和尺寸，处理横竖屏、旋转、原声、单/双声道及无声素材。
- 低分辨率预览、真实 H.264/AAC MP4、持久化任务及逐片段来源记录。
- 验证分辨率、帧数、时长和完整解码后才发布文件；不覆盖已有导出。
- JSON CLI 与 P3 v1 工具入口；证据检索、proposal、异步任务和 HTTP API 详见 [P3 API](docs/api.md)。

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

完整的 Windows 操作、时间线 JSON 例子、备份和排错见 [Docker 使用说明](docs/docker.md)。P1 命令会启动一次容器并退出；运行 P3 本地 API 时使用：

```sh
docker compose run --rm --service-ports video-agent --data /workspace/data serve --addr 0.0.0.0:8090
```

容器内的 `0.0.0.0` 仅用于映射到 Docker 的 loopback 端口；二进制直接运行时 `serve` 只允许 loopback。工具协议、API 与外部 Agent 回放见 [P3 API](docs/api.md) 和 [Agent 调用流程](docs/agent-workflow.md)。

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

下一阶段为 P4：字幕/BGM、Web 适配、对话式交互和中断恢复。P2 的 provider、场景理解与成片质量仍持续迭代。

## 两人协作开发

任务已按 A（媒体与智能）/ B（平台与交互）拆分，包含依赖、模块边界和验收条件。先看 [协作计划](docs/collaboration/README.md)、[GitHub 任务索引](docs/collaboration/github-index.md) 和 [贡献指南](CONTRIBUTING.md)，再认领 Issue。产品仍面向单人本地使用。

完整范围见 [产品定义](docs/product-definition.md)、[需求](docs/requirements.md)、[架构](docs/architecture.md)、[路线图](docs/roadmap.md)、[调研结论](docs/research.md) 和 [验证说明](docs/verification.md)。
