# 验证说明

当前骨架的验证目标是确认“可编译、可测试、编辑协议可重复执行”，不是证明完整的视频导出已经接通。

## 本地命令

```sh
cd /Users/zylar/clip/video-agent
go test ./...
go run ./cmd/video-agent demo
```

`go test ./...` 覆盖资产引用、源时间范围、revision 冲突、锁定片段、幂等重放和插入操作。`demo` 输出一次裁剪后的 `TimelineRevision` 和 `render.Plan` JSON。

## 当前边界

- `render.Plan` 只是确定性的媒体输入计划，不会调用 FFmpeg，也不会产生 MP4。
- `MemoryStore` 只用于协议测试；正式实现要接 GoClip 的 SQLite、任务状态和文件产物。
- `Catalog` 目前是接口，尚未接 ASR、代表帧或向量检索。
- Agent 工具注册表已建立，但 JSON CLI/API 和模型循环留到后续阶段。

## 进入 P1 的条件

先用两段不同帧率、方向和音频布局的真实素材，把 `render.Plan` 编译为 MP4，并用 `ffprobe` 和完整解码检查产物；通过后再把内存存储替换为持久化实现。
