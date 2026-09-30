# 架构草图

```text
已有编程 Agent / 后续内置 Agent
              ↓ JSON CLI/API
        工具契约与参数校验
              ↓
     Go 工程服务：资产、证据、时间线、revision
          ↙                  ↘
   SQLite + 本地文件       持久媒体任务
                               ↓
                     FFmpeg / ASR / Python 构图
                               ↓
                       预览 / 校验 / MP4
```

领域层与媒体层分开。模型只调用受限工具，不能直接写数据库、拼接任意 shell 或修改文件路径。CLI、Web、MCP 和内置 Agent 共用同一套编辑服务。

## 核心对象

- `MediaAsset`：一份不可变输入，含路径引用、内容哈希、时长、尺寸和音轨。
- `Evidence`：来自字幕或画面采样的带时间范围证据。
- `TimelineRevision`：完整不可变时间线，引用多个资产。
- `ClipItem`：资产区间在成片中的位置，包含 `asset_id`、源入出点、输出帧范围和锁定状态。
- `EditOperation`：基于某个 revision 的类型化修改，拥有幂等 ID。
- `MediaJob`：导入、分析、预览和导出任务，拥有可恢复状态。

当前代码先实现这些对象和内存存储，后续将内存存储替换为 GoClip SQLite。

## Agent 运行原则

首版优先由外部编程 Agent 驱动 CLI。内置 Agent 作为后续适配层，建议每次用户请求最多 8 个模型轮次、20 个工具调用，同类参数错误最多自动修正 2 次。长导出由任务系统处理，模型只读取任务状态。

建议工具：`read_project`、`analyze_assets`、`search_segments`、`propose_edit`、`commit_edit`、`restore_revision`、`render_preview`、`export_video`、`get_job`。

编辑提交必须验证资产归属、源区间、时长、锁定约束、base revision 和操作幂等性。模型回复“完成”不代表媒体任务完成；只有真实产物通过元数据和完整解码检查才发布。

