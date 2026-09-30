# Video Agent

面向开发者的单人本地视频 Agent。项目目标是让编程 Agent 通过 CLI/API 读取素材证据、生成可编辑时间线、提交局部修改，并调用可验证的媒体任务导出 MP4。

当前是最小代码骨架，重点验证领域模型和版本化编辑协议。它还没有接入 GoClip 的 FFmpeg、SQLite、ASR 或视觉模型。

## 产品定义

这是一个面向开发者的本地视频 Agent 工具链，不是剪映替代品，也不是通用 Code Agent。Code Agent 负责理解需求和规划步骤，Video Agent 负责通过结构化时间线、版本化编辑和可验证渲染把视频可靠地做出来。完整定位见 [产品定义](docs/product-definition.md)。

## 现在有什么

- 多素材领域模型：`MediaAsset`、`ClipItem`、`TimelineRevision`。
- 版本化编辑：裁剪、替换、删除、插入、移动、锁定。
- 乐观版本检查：旧 revision 提交会被拒绝。
- 操作幂等：相同 operation ID 重复提交会返回原来的 revision。
- 受限工具注册表：为 CLI、内置 Agent 和后续 MCP 共用。
- 确定性渲染计划编译器：把时间线编译成有来源的媒体片段清单。

## 运行

需要 Go 1.22 或更高版本：

```sh
go test ./...
go run ./cmd/video-agent demo
```

`demo` 只在内存中创建两段素材、执行一次裁剪并输出 JSON。它用于验证工程协议，不会读写视频文件。

## 目录

```text
cmd/video-agent/       CLI 入口
internal/domain/       素材、片段、时间线和编辑操作
internal/store/        当前为内存存储，后续接 SQLite
internal/edit/         版本校验、编辑补丁和幂等提交
internal/catalog/      分析证据与语义检索接口
internal/render/       时间线到媒体渲染计划的编译器
internal/agent/        工具契约和注册表
docs/                  产品、架构和实施计划
```

## 下一步

1. 把 `internal/store` 接到 GoClip 的 SQLite 和任务租约。
2. 把 `internal/render` 接到 GoClip 的 FFmpeg/字幕/构图执行器。
3. 增加媒体资产导入、内容哈希、ASR 和代表帧分析。
4. 暴露 JSON CLI/API，让已有编程 Agent 先调用。
5. 在工具协议稳定后，再实现内置 Agent 或 MCP。

完整范围见 [产品定义](docs/product-definition.md)、[需求](docs/requirements.md)、[架构](docs/architecture.md)、[路线图](docs/roadmap.md)、[调研结论](docs/research.md) 和 [验证说明](docs/verification.md)。
