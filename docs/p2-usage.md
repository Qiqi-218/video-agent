# P2 分析闭环使用说明

P2 把视频变成可核对的 `Evidence`：字幕 cue 带源时间戳，代表帧带源区间和本地 JPEG 路径，分析运行记录阶段状态与缓存键。它不是剪辑模型，也不会在没有来源时猜测片段。

## 本地字幕 + 代表帧

先导入项目和素材，然后调用 `analyze`：

```sh
bin/video-agent --data data/demo project create --id demo --name demo
bin/video-agent --data data/demo assets import --project demo --path /workspace/input/video.mp4
printf '%s\n' '{"project_id":"demo","asset_id":"<asset-id>","subtitle_path":"/workspace/input/video.srt","visual":true}' \
  | bin/video-agent --data data/demo tool call --tool analyze --file -
```

SRT 和 VTT 均支持。重复调用会按素材内容哈希、分析器版本、provider、参数、字幕内容哈希和视觉开关复用已完成运行。

## OpenAI-compatible provider

ASR：`VIDEO_AGENT_ASR_BASE_URL`、`VIDEO_AGENT_ASR_MODEL`、`VIDEO_AGENT_ASR_API_KEY`。视觉：`VIDEO_AGENT_VISION_BASE_URL`、`VIDEO_AGENT_VISION_MODEL`、`VIDEO_AGENT_VISION_API_KEY`，也会自动兼容 GoClip 的 `AUTOCLIP_VISION_*` 环境变量。Base URL 可填服务根地址或带 `/v1` 的地址。ASR 使用 `/audio/transcriptions`，视觉使用 `/chat/completions`。

GoClip 的 `AUTOCLIP_TEXT_*` 是文本/规划模型，不是音频转写接口，不能直接当 ASR 使用；没有独立 ASR 时请使用 GoClip 下载的 SRT，或额外配置 `VIDEO_AGENT_ASR_*`。GoClip 网页中保存的密钥位于其加密数据库，Video Agent 不会直接读取或复制密钥。

provider 是可选的；没有密钥时，字幕导入和本地代表帧仍可用。provider 错误会保存为 failed 运行，修复配置后用相同请求重试即可。

## 交给 Code Agent

Code Agent 负责把自然语言目标拆成 `analyze → search → proposal_create → edit_apply → render_submit`。Video Agent 只接受结构化输入，所有片段都必须能回溯到 `Evidence` 的源时间范围。
