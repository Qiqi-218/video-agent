# 对话式剪辑 Agent（工具调用架构）

本文说明 Video Agent 从「固定流水线 + 表单」转向「会自己调用工具的对话 Agent」的设计。
面向实现者和参赛文档撰写者。

---

## 1. 为什么要改：旧结构缺的是什么

改造前的 Web 流程是一条**写死的顺序**：

```
创建项目 → 选文件 → 导入 → 点分析 → 填一句话 → 生成方案 → 确认 → 预览 → 导出
```

每一步都由前端按钮驱动，模型只在其中**一个**位置出现（把用户那句话映射成检索关键词）。
结果是：

- 用户**必须按界面的顺序走**，跳一步就断（例如没点「分析」直接提方案会报 `model_unavailable`）。
- 关键词检索是**逐字子串匹配**。用户说「一分钟以上」里没有任何字幕原话 → 检索为空 → 整个请求被拒绝，
  而界面从不告诉用户字幕里到底有什么词，于是**死循环**（见 PR #4 的修复）。
- 模型**看不到系统状态**。它不知道有哪些项目、素材多长、有没有分析过。

对照 DSH 这类 harness，缺的不是模型能力，而是**运行时**：模型需要能自己决定"下一步做什么"。
DSH 的做法就是把能力拆成工具、给模型一个循环、把每次调用实时展示出来。

---

## 2. 已实现的架构

```
internal/analysis/provider/toolcall.go   模型侧：多轮消息 + tools + tool_calls 解析
internal/agent/toolspec.go               17 个工具的 JSON Schema + 调用入口 + 结果截断
internal/agent/loop.go                   Agent 循环：模型 → 工具 → 回灌 → 直到答完
internal/agent/runtime.go                会话管理、系统提示、并发保护
internal/httpapi/agent.go                SSE 流式接口
internal/httpapi/web/chat.html           对话前端（工具卡片）
```

### 2.1 模型侧：`provider.Chat`

原来 only 有 `Complete(ctx, prompt) string`——一次调用、无历史、无工具。
新增：

```go
type Message struct { Role, Content string; ToolCalls []ToolCall; ToolCallID, Name string }
type ToolDefinition struct { Name, Description string; Parameters map[string]any }
type ChatResult struct { Content string; ToolCalls []ToolCall; Finish string }

func (p OpenAIText) Chat(ctx, messages []Message, tools []ToolDefinition) (ChatResult, error)
```

要点：

- 工具按 OpenAI 线格式包一层 `{"type":"function","function":{...}}`，缺这层模型不会调用。
- `tool_choice: "auto"`，由模型决定用不用工具。
- 有工具时才发 `tools` 字段；没有工具时保持原来的纯补全行为，老调用方不受影响。
- `AssistantMessage()` 会把 assistant 那一轮（含 `tool_calls`）**原样回放**进下一次请求。
  漏掉它，provider 会因为「工具结果找不到对应的调用」而拒绝。

### 2.2 工具目录：`ToolSpecs()`

**关键发现：工具早就存在了，只是从没交给模型。**

`agent.Service.call()` 本来就有 17 个 action（`project_create`、`assets_import`、`analyze`、
`search`、`timeline_create`、`edit_apply`、`render_submit` …），每个都有完整的入参校验和错误码，
但只通过 HTTP 和 CLI 暴露。同时 `agent.Registry`（带 `InputSchema`）**从未被任何代码使用**，是死代码。

所以这一步是**接线**而不是重写：

- `ToolSpecs()` 为每个 action 声明 name / description / JSON Schema。
- `ModelTools()` 投影成模型可见的格式，**只发 name/description/parameters**。
- `RunTool()` 复用已有的 `Service.Call`，模型拿到的结果和 CLI 完全同一套语义。

**防漂移测试**：`TestToolSpecsMatchCallableActions` 双向断言——
声明的工具必须可调用，可调用的 action 必须已声明。少了任何一边都编译不过测试。

### 2.3 Agent 循环：`Runner.Turn`

```
for step := 1..MaxSteps:
    result = model.Chat(history, tools)
    history += assistant(result)          // 必须带 tool_calls
    if result 没有工具调用: return          // 答完了
    for call in result.ToolCalls:
        emit(tool_start)
        output = tools.RunTool(call)
        emit(tool_end)
        history += tool(call.id, output)
```

设计取舍：

- **步数上限**（默认 12）。没有上限的工具循环就是成本事故；撞上限是**正常结束**并告知用户，
  而不是崩掉——此时历史里已经有做过的全部工作。
- **每次调用都发事件**。前端能实时显示"正在调用 search"、"search 返回了…"，
  这是"看得见的 agent"，也是评分里「作品呈现」的抓手。
- **失败回灌给模型**，不是抛给用户。模型看到 `{"ok":false,"error":{...}}` 会自己换做法——
  实测中它确实这么做了（见 §4）。

### 2.4 结果截断（必须做的事）

一次 `analyze` 在 5 分钟素材上返回 ~26KB、136 条证据；两小时素材会直接撑爆上下文。
`RunTool` 里统一截断到 16KB，并且**告诉模型它被截断了**、让它用更精确的参数再查：

```json
{"truncated":true,"notice":"结果过长已截断…请改用更精确的参数…","partial_json":"…"}
```

片段的 JSON 再编码会让引号和反斜杠膨胀，所以原始片段只保留 11KB，给转义留出余量。

### 2.5 昂贵操作需要用户确认（结构强制，不靠提示词）

**实测教训**：只写在系统提示里（"渲染前先问用户"）**不管用**——模型照样直接调了 `render_submit`。
提示词不是权限边界。

所以改成在循环里硬拦：

```go
var expensiveTools = map[string]string{
    "render_submit": "渲染会真实消耗时间并写出文件，需要用户先明确同意",
}
```

不同意时不执行，而是回一个结构化拒绝让模型转去征求同意：

```json
{"ok":false,"error":{"code":"confirmation_required","message":"…请先说明打算剪成什么样，等用户确认后再调用本工具。"}}
```

"同意"的判定（`History.UserConsented`）刻意保守：**必须先出现过确认询问，之后用户才说"可以/好的/确认"**。
单独一句"好的"不构成授权。实测两轮对话验证：
先问 → 拦住；用户答"好的，确认导出" → 放行。

### 2.6 SSE 流式接口

| 方法 | 路径 | 作用 |
|---|---|---|
| GET | `/chat` | 对话界面 |
| GET | `/v1/agent/config` | 模型是否就绪、工具数量（**不回显密钥**） |
| GET | `/v1/agent/sessions` | 会话列表（侧栏） |
| GET | `/v1/agent/sessions/{id}` | 历史回放，刷新页面不丢上下文 |
| POST | `/v1/agent/chat` | 执行一轮，SSE 流式返回 |

事件类型：`session` / `agent`（内含 `step`、`text`、`tool_start`、`tool_end`、`error`、`done`）/ `end`。
每 15 秒发一次 keep-alive 注释帧，防止中间层掐掉空闲连接。

### 2.7 前端

`chat.html` 是一个自包含页面（无构建、无依赖），对应 DSH 的布局思路：

- **左侧栏**：会话列表 + 新建对话 + 当前模型与工具数量。
- **主区**：消息流；**每次工具调用渲染成一张可展开的卡片**（工具名 + 参数 + 状态 + JSON 结果），
  失败自动展开。这是"agent 真的调了工具"最直观的证据。
- **输入区**：Enter 发送 / Shift+Enter 换行；运行中按钮变「停止」，可中断。
- 顶部实时显示到第几步、正在调用哪个工具。

---

## 3. 这一步顺带补上的能力

- `Store.Projects()`：**原来没有任何列举项目的方法**。agent 只能靠猜 id，
  实测中它对 `project_get` 传空参数拿到 not_found，然后**误判"系统里没有项目"**并凭空建了一个。
  补上 `project_list` 工具，并附带回 `asset_count`（否则模型要逐个项目调 `assets_list`，浪费步数）。

---

## 4. 实测记录

真实模型 `qwen-plus` + 真实素材（B 站视频，299.8 秒 / 1920×1080）。

**一次完整剪辑（8 步，全程自主）**

```
project_list → project_get → assets_list → analyze(visual:true)
→ search("数学") → timeline_create → render_submit → 回答
```

产物 `math-1min.mp4`：11.6 MB，H.264 + AAC，`duration=60.000000`。

**自愈行为**：模型先调 `project_create` 不带 `id` → 收到
`project requires id and name` → **自己读出错误含义并补上 `id` 重试成功**。
这正是"失败回灌给模型"的价值：不需要把每种错误都写进提示词。

**确认门控**

| 轮次 | 输入 | 结果 |
|---|---|---|
| 1 | 「把 timeline-math-001 导出成 MP4」 | 被拦，回 `confirmation_required`，改口询问「请问是否确认导出这个 60 秒的 MP4？」 |
| 2 | 「好的，确认导出」 | 放行，提交渲染任务 |

**界面**（真实 Chrome）：配置徽标「模型就绪」、工具 17、点建议问题后渲染出用户气泡 +
`project_list` 工具卡片（状态"完成"、可展开 JSON）+ 助手归纳回答。

---

## 5. 还没做的（按优先级）

1. **会话持久化**。现在会话在内存里，服务重启即丢失。DSH 用 JSONL + SQLite 落盘；
   这里应该复用已有的 SQLite（新建 `sessions` / `messages` 表）。
2. **上下文压缩**。长对话迟早撑爆窗口。DSH 有 `dsh-compaction`；这里至少要做工具结果修剪
   （已有截断，但历史里的旧结果可以进一步摘要）。
3. **并行工具调用**。同一轮里的多个无依赖调用现在串行执行。DSH 有
   `isConcurrencySafe` 标记 + `maxParallelToolCalls`。
4. **文件上传进对话**。现在导入素材仍然走经典页面；对话里应该能直接拖文件进来
   （对应 DSH 的 attachment）。
5. **工作区侧栏**。DSH 有文件树/终端/产物面板；这里对应"素材列表 + 时间线预览 + 成片"。
6. **语义检索**。`search` 仍是子串匹配，用户说"讲数学的片段"能命中是因为字幕里恰好有"数学"。
   真正的主题检索需要向量或让模型基于证据摘要选片。

---

## 6. 与 DSH 的对应关系

| DSH | 本项目 | 状态 |
|---|---|---|
| `dsh-agent-loop` | `agent.Runner.Turn` | ✅ |
| `dsh-tools` + `defineTool` | `ToolSpecs` / `RunTool` | ✅（含防漂移测试） |
| `dsh-tool-*` 各工具包 | 17 个剪辑工具 | ✅ |
| `tools/execute` 前置/后置策略 | `confirmationRequired` 门控 | ✅（最小实现） |
| `presentCall` / `presentResult` | 前端工具卡片 | ✅（前端渲染而非工具声明） |
| `dsh-session-persistence` | — | ❌ 待做 |
| `dsh-compaction` | 结果截断 | ⚠️ 部分 |
| `maxParallelToolCalls` | — | ❌ 待做 |
| `dsh-client-ui-chat` | `web/chat.html` | ✅ |

---

## 7. 本地运行

```powershell
. E:\huabei\Activate-Venv.ps1          # 载入 AUTOCLIP_TEXT_* 等模型配置
cd E:\huabei\video-agent
go build -o bin\video-agent.exe ./cmd/video-agent
.\bin\video-agent.exe --data .\data serve --addr 127.0.0.1:8090
```

- 对话界面：<http://127.0.0.1:8090/chat>
- 经典剪辑台：<http://127.0.0.1:8090/>

环境变量是**进程级**的：新开终端必须先激活，否则服务读不到模型配置，
界面上会显示「模型未配置」横幅。
