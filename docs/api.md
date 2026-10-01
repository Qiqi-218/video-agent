# P3 本地 Agent API（v1）

P3 将现有的确定性编辑引擎包装为本地、受限的工具 API。它不是 MCP：外部 Code Agent 可以用 HTTP 或 JSON CLI 调用它，但不会获得任意 shell、数据库或文件系统权限。

启动服务（默认只监听 loopback）：

```sh
bin/video-agent --data data/demo serve --addr 127.0.0.1:8090
```

所有工具结果都是下列 envelope；业务代码应使用 `error.code`，不要按错误文案分支。

```json
{"api_version":"v1","ok":true,"result":{}}
```

错误码：`invalid_request`、`not_found`、`no_match`、`model_unavailable`、`timeout`、`cancelled`、`revision_conflict`、`internal`。

## 入口

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/v1/health` | 本地服务健康状态 |
| `GET` | `/v1/tools` | 可调用工具名称 |
| `POST` | `/v1/tools/{tool}` | 调用工具，body 是 JSON 输入 |
| `GET` | `/v1/jobs/{id}` | 读取异步导出任务 |
| `POST` | `/v1/jobs/{id}/cancel` | 取消 queued/running 导出 |
| `GET` | `/v1/artifacts/{job_id}` | 读取成功任务的 MP4，支持 HTTP Range |

产物读取只允许数据库中已完成任务登记、并位于 `<data>/exports/` 内的常规文件；路径穿越和 P1 任意 `--output` 位置不会被 API 暴露。

## 工具

`project_create`、`project_get`、`assets_import`、`assets_list`、`timeline_create`、`timeline_get`、`timeline_history`、`edit_apply` 直接映射已有 P1 领域对象，继续执行资产归属、revision、锁定和幂等操作 ID 校验。

新增 P3 工具：

| 工具 | 输入重点 | 结果 |
|---|---|---|
| `evidence_add` | 完整 `Evidence`（项目、素材、源微秒范围及字幕/视觉内容） | 持久化、带素材内容哈希的来源证据 |
| `analyze` | `project_id`、`asset_id`、可选 `subtitle_path`、`visual`、`provider`、`parameters` | 执行/复用分析运行，返回 `run`（阶段状态、缓存键）和带来源的 `evidence`；可用本地 SRT/VTT，无字幕且未配置 ASR 时返回 `model_unavailable` |
| `search` | `project_id`、`query`、可选 `asset_ids`、`limit` | 稳定排序的证据命中；无命中为 `no_match` |
| `proposal_create` | `timeline_id`、`query`、`limit` | 未提交的 `EditProposal`，含连续 revision 的插入操作 |
| `render_submit` | `timeline_id`、可选 `revision`、`preview`、`filename` | 立即返回 `queued` job；文件名只能是 `.mp4` 基名 |
| `jobs_get`、`jobs_list`、`jobs_cancel` | 任务 ID（list 无输入） | 查询、列表或取消任务 |

P2 使用 OpenAI-compatible HTTP 接口作为可选 provider。配置 `VIDEO_AGENT_ASR_BASE_URL`、`VIDEO_AGENT_ASR_MODEL`、`VIDEO_AGENT_ASR_API_KEY`（或兼容的 `VIDEO_AGENT_TEXT_*`）启用转写；配置 `VIDEO_AGENT_VISION_*` 启用逐帧描述。provider 失败会持久化为 failed，不会伪装成完成。

本地字幕与代表帧示例：

```sh
printf '%s\n' '{"project_id":"demo","asset_id":"asset-1","subtitle_path":"/workspace/input/video.srt","visual":true}' \
  | bin/video-agent --data data/demo tool call --tool analyze --file -
```

## JSON CLI

CLI 用同一服务而非另一套业务规则：

```sh
bin/video-agent --data data/demo tool list
printf '%s\n' '{"id":"demo"}' | bin/video-agent --data data/demo tool call --tool project_get --file -
```

`tool call` 的成功与失败都写出 v1 envelope；失败退出码非零。异步任务需要运行中的 `serve` 进程来执行，避免 CLI 进程退出后无人消费队列。
