# CubeRouter 图片与视频模型 API 使用说明

本文按当前代码的实际行为整理图片、视频模型 API 的调用方式与可接受参数。仓库中的 `docs/openapi/relay.json` 含一份示例规格，但部分字段（例如视频 `VideoRequest` 的 `width`/`height`/`fps`）并未被统一任务接口读取，请以本文和代码为准。

## 目录

- [1. 通用约定](#1-通用约定)
- [2. 图片模型 API](#2-图片模型-api)
- [3. 视频模型 API](#3-视频模型-api)
- [4. 计费与参数边界](#4-计费与参数边界)
- [5. 常见坑](#5-常见坑)
- [6. 代码索引](#6-代码索引)

## 1. 通用约定

- **Base URL**：`https://<站点域名>/v1`
- **鉴权**：`Authorization: Bearer <API Key>`
  - 图片、视频接口只认 `Authorization`；
  - `x-api-key`、`x-goog-api-key` 仅在 `/v1/messages`、`/v1/models` 等特定路径生效。
- **模型路由**：`model` 必须在令牌可用分组/渠道中；请求体顶层还可带 `group` 指定分组。
- **同步与异步**：
  - 图片接口是同步的，直接返回结果；
  - 视频接口是异步任务，先提交拿 `task_id`，再轮询状态，最后取视频内容。
- **错误格式**：
  - 图片：OpenAI 风格 `{"error": {"message": ..., "type": ..., "code": ...}}`
  - 视频任务：`{"code": "<错误码>", "message": "<错误信息>", "data": null}`，HTTP 状态码表示类别

### 1.1 如何选择入口

| 场景 | 推荐入口 |
|---|---|
| 文生图 | `POST /v1/images/generations` |
| 图片编辑/图生图 | `POST /v1/images/edits` |
| 通用视频生成 | `POST /v1/video/generations` + `GET /v1/video/generations/:task_id` |
| 按 OpenAI 官方 Video API 习惯 | `POST /v1/videos` + `GET /v1/videos/:task_id` |
| 火山 Ark / Seedance 原生协议 | `POST /v1/videos/generations/tasks` |
| 需要供应商完整原生参数 | `POST /v1/tasks/:plugin_key`，或 `/kling/...`、`/jimeng/...` |
| 前端登录态（Media Studio / Playground） | `/pg/images/*`、`/pg/video/*` |

## 2. 图片模型 API

### 2.1 端点

| 端点 | 用途 | 鉴权 | 说明 |
|---|---|---|---|
| `POST /v1/images/generations` | 文生图 | API Key | 标准 OpenAI Images 风格 |
| `POST /v1/images/edits` | 图片编辑（JSON 或 multipart） | API Key | 参考图可为 base64 或文件 |
| `POST /v1/edits` | 编辑别名 | API Key | 与 `/v1/images/edits` 等价 |
| `POST /v1/images/variations` | — | API Key | 返回 501，不支持 |
| `POST /pg/images/generations` | 前端 Media Studio 文生图 | 登录会话 | 与 API 共用同一图片处理链 |
| `POST /pg/images/edits` | 前端 Media Studio 编辑 | 登录会话 | 同上 |

路由定义：`router/relay-router.go:135`、`router/relay-router.go:69`、`relay/constant/relay_mode.go:66`。

### 2.2 请求参数

JSON 请求体按 `relaykit/dto/openai_image.go:17` 的 `ImageRequest` 解析：

| 参数 | 类型 | 说明 |
|---|---|---|
| `model` | string | **必填**；JSON 路径不传会直接返回 400 |
| `prompt` | string | 提示词；网关未强制，但上游通常必需 |
| `n` | uint | 生成数量，默认 1，**上限 128**（`dto.MaxImageN`） |
| `size` | string | 如 `1024x1024`；必须是小写 `x`，不能使用 `×` |
| `quality` | string | dall-e：`standard`/`hd`；gpt-image：`auto`/`high`/`medium`/`low`；Gemini 会映射为 `1K`/`2K` |
| `response_format` | string | `url` 或 `b64_json` |
| `stream` | bool | OpenAI 兼容渠道支持 SSE 图片流；其他渠道一般按普通 JSON 处理 |
| `style` | any | 原样透传 |
| `user` | any | 原样透传 |
| `image` | string / string[] | 编辑参考图；base64 data URL 或 URL，取决于渠道 |
| `images` | array | 编辑参考图数组（OpenAI 官方 JSON 方言；图片编辑渠道 63 会拒绝） |
| `mask` | any | 蒙版，原样透传 |
| `background` | any | gpt-image 背景参数，原样透传 |
| `moderation` | any | 原样透传 |
| `output_format` | any | 原样透传 |
| `output_compression` | any | 原样透传 |
| `partial_images` | any | 流式部分图，原样透传 |
| `input_fidelity` | any | 原样透传 |
| `watermark` | bool | 部分渠道支持 |
| `watermark_enabled` | any | 智谱等渠道方言 |
| `user_id` | any | 部分渠道方言 |
| `num_inference_steps` | int | 扩散步数（图片编辑渠道等） |
| `seed` | int64 | 随机种子（部分渠道） |
| `true_cfg_scale` | float64 | CFG 系数（部分渠道） |
| `extra_fields` | any | 原样透传 |
| 其他未知字段 | — | 进入 `Extra`，**不会**被通用 OpenAI 适配器自动平铺转发 |

dall-e 系列会额外校验：

- `dall-e-2`/`dall-e`：`size` 只能是 `256x256`、`512x512`、`1024x1024`，缺省 `1024x1024`
- `dall-e-3`：`size` 只能是 `1024x1024`、`1024x1792`、`1792x1024`，缺省 `1024x1024`；`quality` 缺省 `standard`
- `gpt-image-1`：`quality` 缺省 `auto`

代码：`relay/helper/valid_request.go:182`。

### 2.3 Provider 扩展参数

未知顶层字段只对特定适配器有意义：

#### 阿里 DashScope（qwen-image 等）

可传顶层 `parameters` 和 `input`：

```json
{
  "model": "qwen-image-plus",
  "parameters": {
    "size": "1328*1328",
    "n": 1,
    "negative_prompt": "",
    "prompt_extend": true,
    "watermark": false,
    "seed": 42,
    "steps": "40",
    "scale": "5",
    "thinking_mode": false,
    "enable_sequential": false,
    "bbox_list": [],
    "color_palette": []
  },
  "input": {
    "prompt": "一副典雅庄重的对联",
    "negative_prompt": "",
    "messages": [
      {
        "role": "user",
        "content": [{ "text": "一副典雅庄重的对联" }]
      }
    ]
  }
}
```

注意：

- `parameters.size` 使用 `*`（如 `1328*1328`）
- `parameters.n` 同样受 1–128 限制
- `input.messages` 是同步图片模型方言，异步模型使用 `input.prompt`

代码：`relay/channel/ali/image.go:24`、`relay/channel/ali/dto.go:173`。

#### MiniMax

可传：

- `prompt_optimizer`：bool
- `aspect_ratio`：如 `1:1`、`16:9`

同时 `size` 会被映射为宽高比。代码：`relay/channel/minimax/image.go:42`。

#### Gemini / Vertex

- `n` → `sampleCount`
- `size` → `aspectRatio`
- `quality` → `imageSize`（`1K`/`2K`）

代码：`relay/channel/gemini/adaptor.go:66`。

#### Image Edit (Base64) 渠道 63

该渠道只接受 `/v1/images/edits` 的 **JSON** 请求：

- `image` 必须是 base64（单个字符串或字符串数组），支持 `data URL`
- 拒绝 `images: [{"image_url": ...}]`
- 拒绝 `http(s)` URL
- 拒绝 multipart

代码：`relay/channel/imageedit/adaptor.go:81`。

### 2.4 multipart 图片编辑

`POST /v1/images/edits` 使用 `multipart/form-data` 时，网关解析这些字段：

| 字段 | 说明 |
|---|---|
| `model` | 必填 |
| `prompt` | 提示词 |
| `n` | 1–128，默认 1 |
| `quality` | 质量 |
| `size` | 尺寸 |
| `stream` | 是否流式 |
| `image` / `image[]` / `image[i]` | 参考图文件；OpenAI 渠道支持多种写法 |
| `watermark` | 部分渠道支持 |

OpenAI 渠道会重建 multipart 并转发全部非文件字段；其他渠道只取自己支持的字段。

代码：`relay/helper/valid_request.go:182`、`relay/channel/openai/adaptor.go:523`。

### 2.5 响应

同步 JSON：

```json
{
  "created": 1730000000,
  "data": [
    {
      "url": "https://...",
      "b64_json": "",
      "revised_prompt": "..."
    }
  ],
  "metadata": {}
}
```

- `url` / `b64_json` 由 `response_format` 决定
- `metadata` 部分渠道会带回上游原始响应（如阿里）
- 流式时返回 SSE；事件类型由上游 `type` 字段重建，结束时发送 `data: [DONE]`

代码：`relaykit/dto/openai_image.go:190`、`relay/channel/openai/relay_image.go`。

### 2.6 示例

```bash
# 文生图
curl "$BASE/v1/images/generations" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-1",
    "prompt": "a calico cat playing piano",
    "n": 1,
    "size": "1024x1024",
    "response_format": "b64_json"
  }'

# 编辑：base64 内联（图片编辑渠道 63 必须用这种）
curl "$BASE/v1/images/edits" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen-image-edit",
    "prompt": "把背景换成森林",
    "image": "data:image/png;base64,...."
  }'

# 编辑：multipart 上传（OpenAI 兼容渠道）
curl "$BASE/v1/images/edits" \
  -H "Authorization: Bearer $KEY" \
  -F model=gpt-image-1 \
  -F prompt="把背景换成森林" \
  -F image=@ref.png \
  -F n=1
```

## 3. 视频模型 API

### 3.1 端点矩阵

| 端点 | 用途 | 请求体 | 响应 |
|---|---|---|---|
| `POST /v1/video/generations` | **统一任务提交**（推荐入口） | 统一任务体 `TaskSubmitReq` | `task_id` / OpenAI Video 形状 |
| `GET /v1/video/generations/:task_id` | 统一任务查询 | — | `{code, message, data: TaskDto}` |
| `POST /v1/videos` | OpenAI 官方 Video API 创建 | JSON 或 multipart，由插件解码 | OpenAI Video 对象 |
| `GET /v1/videos/:task_id` | OpenAI Video 查询 | — | OpenAI Video 对象 |
| `GET/HEAD /v1/videos/:task_id/content` | 取视频字节流 | — | `video/mp4` 等 |
| `POST /v1/videos/:video_id/remix` | 基于已有视频 remix | 统一任务体 | 新任务 |
| `POST /v1/videos/generations/tasks` | 火山 Ark 风格提交（**仅 doubao/astraflow**） | Ark `content[]` | Ark 任务对象 |
| `GET /v1/videos/generations/tasks/:task_id` | Ark 风格查询 | — | Ark 任务对象 |
| `POST /v1/tasks/:plugin_key` | 任务插件原生提交 | 各插件原生 body | 插件原生响应 / 任务信息 |
| `GET /v1/tasks/:task_id` | 通用任务摘要 | — | `task_id/status/progress/fail_reason` |
| `GET /v1/tasks/:task_id/artifacts` | 产物列表 | — | `artifacts[]` + `content_url` |
| `GET/HEAD /v1/tasks/:task_id/artifacts/:artifact_key/content` | 取产物内容 | — | 文件流 |
| `POST /kling/v1/videos/text2video` | 可灵官方格式 | 可灵官方 body | 可灵格式 |
| `POST /kling/v1/videos/image2video` | 可灵图生视频 | 可灵官方 body | 可灵格式 |
| `GET /kling/v1/videos/{text2video\|image2video}/:task_id` | 可灵查询 | — | 可灵格式 |
| `POST /jimeng/?Action=...&Version=...` | 即梦官方格式 | 即梦官方 body | 即梦格式 |
| `POST /pg/video/generations` | 前端登录态提交 | 统一任务体 | 任务信息 |
| `GET /pg/video/generations/:task_id` | 前端登录态查询 | — | 任务信息 |

路由定义：`router/video-router.go`、`router/task-router.go`、`router/task-plugin-protocol-router.go`。

### 3.2 统一任务体（`/v1/video/generations`）

定义在 `relay/common/relay_info.go:1040` 的 `TaskSubmitReq`：

| 字段 | 类型 | 说明 |
|---|---|---|
| `model` | string | 提交时必填（remix 可省略，从源任务推导） |
| `prompt` | string | **统一入口必填**；doubao/astraflow 的 Ark `content[]` 里有 text 项时可回填 |
| `image` | string | 单张参考图（URL 或 base64） |
| `images` | string[] | 多张参考图 |
| `content` | array | **仅 doubao/astraflow** 接受；Ark 内容数组 |
| `size` | string | 如 `1280x720`，适配器会归一为分辨率档 |
| `resolution` | string | 如 `480p`/`720p`/`1080p`/`4k` |
| `ratio` | string | 宽高比，如 `16:9`、`9:16`、`1:1`、`adaptive` |
| `duration` | int 或数字字符串 | 时长（秒） |
| `seconds` | string | 时长（秒）；**统一任务体里必须是字符串**，如 `"5"` |
| `input_reference` | string | 参考图；主要给 OpenAI Video/插件路径用 |
| `metadata` | object 或 JSON 字符串 | 厂商参数扩展层；详见下文 |

`content[]` 元素结构（`relay/common/relay_info.go:1031`）：

```json
{
  "type": "text | image_url | video_url | audio_url",
  "text": "...",
  "image_url": { "url": "https://..." },
  "video_url": { "url": "https://..." },
  "audio_url": { "url": "https://..." },
  "role": "first_frame | last_frame | reference_image"
}
```

#### 关键注意事项

- **未知顶层字段会被丢弃**：`width`、`height`、`fps`、`seed`、`n` 等字段不在 `TaskSubmitReq` 中，统一入口不会读取。要传给上游请放进 `metadata`（是否生效取决于渠道/插件）。
- `prompt` 在统一入口由网关强制校验；即使图生视频也必须给 prompt（原生 Kling/插件路由除外）。
- `duration`/`seconds` 上限为 **3600 秒**（`relaycommon.MaxTaskDurationSeconds`）。
- `metadata.model` 不能用来改模型；Go 适配器会直接删除 metadata 中的 `model`。
- `metadata` 既接受对象，也接受 JSON 字符串。
- 动作由适配器自动推导：有参考图 → `image_to_video`，否则 `text_to_video`；remix 走单独动作。

代码：`relay/common/relay_utils.go:325`。

### 3.3 各平台参数映射

#### A. doubao / astraflow（Go 适配器，Ark 风格）

`metadata` 会作为覆盖层合并进上游 Ark 请求体，可用字段：

| 字段 | 说明 |
|---|---|
| `content` | 直接覆盖/追加 Ark content 数组 |
| `callback_url` | 回调地址 |
| `return_last_frame` | 是否返回末帧 |
| `service_tier` | 服务档位 |
| `execution_expires_after` | 执行过期时间 |
| `generate_audio` | 是否生成音频 |
| `draft` | 草稿模式 |
| `tools` | 工具数组 |
| `safety_identifier` | 安全标识 |
| `priority` | 优先级 |
| `resolution` | 分辨率档 |
| `ratio` | 宽高比 |
| `duration` | 时长 |
| `frames` | 帧数 |
| `seed` | 随机种子 |
| `camera_fixed` | 固定镜头 |
| `watermark` | 水印 |

优先级：

- `duration`：`metadata.duration` > 顶层 `duration` > `seconds`
- `resolution`：`metadata.resolution` > 顶层 `resolution` > `size` 归一
- `ratio`：`metadata.ratio` > 顶层 `ratio` > 默认 `adaptive`

`images` 会转成 `content` 里的 `image_url` 项；`prompt` 会合成/补进 `text` 项。直传 `content` 时其中 `text` 项即提示词。

代码：`relay/channel/task/doubao/adaptor.go:299`、`relay/channel/task/astraflow/adaptor.go:241`。

**astraflow 的 Kling 家族**（模型名以 `kling` 开头）使用另一套方言：

```json
{
  "model": "kling-v3",
  "input": {
    "prompt": "...",
    "img_url": "...",
    "video_url": "..."
  },
  "parameters": {
    "kling_v3_type": "t2v | i2v | motion_control",
    "image": "...",
    "image_tail": "...",
    "mode": "std | pro | 4k",
    "sound": "on | off",
    "aspect_ratio": "16:9 | 9:16 | 1:1",
    "duration": 5,
    "character_orientation": "...",
    "keep_original_sound": "...",
    "multi_shot": true,
    "shot_type": "...",
    "multi_prompt": [],
    "image_list": [],
    "video_list": []
  }
}
```

**astraflow 的 wan3 家族**（模型名以 `wan3.` 开头）：

```json
{
  "model": "wan3.0-video",
  "input": {
    "prompt": "...",
    "media": [
      { "type": "first_frame", "url": "..." },
      { "type": "last_frame", "url": "..." },
      { "type": "reference_image", "url": "..." }
    ]
  },
  "parameters": {
    "resolution": "720p",
    "ratio": "16:9",
    "duration": 5
  }
}
```

`reference_video` / `reference_audio` 等只能通过 `metadata.media` 下发。

代码：`relay/channel/task/astraflow/adaptor.go:645`、`relay/channel/task/astraflow/adaptor.go:874`。

#### B. JS 任务插件平台

统一入口 `metadata` 是否整包透传取决于插件：

| 插件 key | 默认模型（参考） | 主要参数 |
|---|---|---|
| `doubao` | Seedance 1.0/1.5/2.0/2.5 系列 | `metadata` 整体合并；`content`、`seconds`、`duration` |
| `kling` | `kling-v1`、`kling-v1-6`、`kling-v2-master` | `prompt`、`image`、`metadata`（整体合并），常用 `mode`、`duration`、`aspect_ratio`、`cfg_scale`、`image_tail`、`camera_control` 等 |
| `sora` | `sora-2`、`sora-2-pro` | `prompt`、`seconds`/`duration`、`size`、`metadata`；multipart 支持 `input_reference` |
| `vidu` | `viduq3-mix`、`viduq3-turbo`、`viduq3`、`viduq2-pro`、`viduq2`、`viduq1`、`vidu2.0`、`vidu1.5` | `images`、`prompt`、`duration`、`resolution`；`metadata` 整体合并 |
| `alibaba` | wan2.7/2.5/2.2、wanx2.1 系列 | `prompt`、`image`/`images`/`input_reference`、`size`、`resolution`、`duration`；`metadata.input` / `metadata.parameters` |
| `hailuo` | `MiniMax-H3`、Hailuo 2.3/02、T2V/I2V-01 系列、S2V-01 | `prompt`、`duration`、`resolution`；`metadata` 白名单：`prompt_optimizer`、`fast_pretreatment`、`callback_url`、`aigc_watermark`、`first_frame_image`、`last_frame_image`、`subject_reference` |
| `jimeng` | `jimeng_vgfm_t2v_l20` | `req_key`、`binary_data_base64`、`image_urls`、`prompt`、`seed`、`aspect_ratio`、`frames` |
| `google` | Veo 3.0/3.1 系列 | `prompt`、`duration`（4/6/8）、`size`/`metadata.resolution`；`metadata` 整体合并 |
| `vertex-ai` | Veo 3.0/3.1 系列 | 同上，另支持 `generate_audio` |
| `sunoapi` | `suno_music`、`suno_lyrics` | 音频任务，不属视频 |

插件源码：`plugins/tasks/*/plugin.js`；插件协议文档：`docs/plugin-api/v1.md`。

#### C. `/v1/videos`（OpenAI Video API）

- 支持 **JSON 或 multipart**
- multipart 常用字段：`model`、`prompt`、`seconds`、`size`、`input_reference`（文件）
- 请求由插件 `decodeRequest` 解码；不同插件接受的字段不同
- `seconds` 一般 1–3600
- 例如 doubao 插件要求参考图/视频以 URL 放在 `metadata.content`，不接受 multipart 文件
- 提交返回 OpenAI Video 对象：

```json
{
  "id": "vt_xxx",
  "object": "video",
  "model": "sora-2",
  "status": "queued",
  "progress": 0,
  "created_at": 1730000000,
  "completed_at": 0,
  "seconds": "8",
  "size": "720x1280",
  "error": null,
  "metadata": {}
}
```

#### D. `/v1/videos/generations/tasks`（Ark 风格）

仅 **doubao / astraflow** 渠道可用，其他渠道直接 400。请求体：

```json
{
  "model": "doubao-seedance-2-0-260128",
  "content": [
    { "type": "text", "text": "宇航员在月球漫步" },
    { "type": "image_url", "image_url": { "url": "https://..." }, "role": "first_frame" }
  ],
  "duration": 5,
  "resolution": "720p",
  "ratio": "16:9"
}
```

响应（`dto/ark_video.go:20`）：

```json
{
  "id": "vt_xxx",
  "model": "doubao-seedance-2-0-260128",
  "status": "queued | running | succeeded | failed | expired",
  "created_at": 1730000000,
  "updated_at": 1730000001,
  "content": { "video_url": "..." },
  "output": { "duration": 5 },
  "usage": { "completion_tokens": 12345 },
  "error": { "code": "...", "message": "..." }
}
```

#### E. `/v1/tasks/:plugin_key`（任务插件原生入口）

- 提交：`POST /v1/tasks/kling`、`/v1/tasks/doubao`、`/v1/tasks/vidu`、`/v1/tasks/alibaba` 等，`:key` 是插件 key
- 请求体是**插件定义的原生 body**，因此可以使用该供应商的完整参数集
- 查询：`GET /v1/tasks/:task_id`（这里的 `:key` 是任务 ID）
- 产物：`GET /v1/tasks/:task_id/artifacts`、`GET /v1/tasks/:task_id/artifacts/:artifact_key/content`

#### F. Kling / Jimeng 官方入口

- `POST /kling/v1/videos/text2video`、`/kling/v1/videos/image2video`
- `POST /jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31`
- 请求体是供应商官方格式；中间件会把整个原始 body 放进 `metadata`，再转成统一任务
- 因此官方文档里的 `model_name`、`image_tail`、`mode`、`duration`、`aspect_ratio`、`cfg_scale` 等都能用

代码：`middleware/kling_adapter.go`、`middleware/jimeng_adapter.go`。

### 3.4 查询与状态

| 入口 | 状态字段 | 成功结果 |
|---|---|---|
| `/v1/video/generations/:task_id` | `SUBMITTED` / `QUEUED` / `IN_PROGRESS` / `SUCCESS` / `FAILURE` | `data.result_url` |
| `/v1/videos/:task_id` | `queued` / `in_progress` / `completed` / `failed` | 需再调 `/content` |
| `/v1/videos/generations/tasks/:task_id` | `queued` / `running` / `succeeded` / `failed` / `expired` | `content.video_url` |
| `/v1/tasks/:task_id` | 内部状态 | `result_url` |

通用查询返回：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "vt_xxx",
    "platform": "54",
    "status": "SUCCESS",
    "progress": "100%",
    "fail_reason": "",
    "result_url": "https://...",
    "created_at": 1730000000,
    "finished_at": 1730000030,
    "data": {}
  }
}
```

视频内容接口 `/v1/videos/:task_id/content` 支持：

- `Authorization: Bearer <API Key>`
- dashboard 会话 / PAT
- 带签名的 `?access=...` capability（供 `<video>` 标签直接播放）

代码：`relay/relay_task.go:474`、`controller/video_proxy.go:63`。

### 3.5 示例

```bash
# 统一提交
curl "$BASE/v1/video/generations" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-260128",
    "prompt": "宇航员在月球上漫步",
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9",
    "metadata": { "seed": 42, "generate_audio": true }
  }'

# 轮询
curl "$BASE/v1/video/generations/vt_xxx" \
  -H "Authorization: Bearer $KEY"

# OpenAI Video 风格（multipart）
curl "$BASE/v1/videos" \
  -H "Authorization: Bearer $KEY" \
  -F model=sora-2 \
  -F prompt="cute cat dance" \
  -F seconds=8 \
  -F input_reference=@ref.png

# Ark 风格
curl "$BASE/v1/videos/generations/tasks" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "doubao-seedance-2-0-260128",
    "content": [
      { "type": "text", "text": "宇航员在月球上漫步" },
      { "type": "image_url", "image_url": { "url": "https://..." }, "role": "first_frame" }
    ],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9"
  }'

# 可灵官方格式
curl "$BASE/kling/v1/videos/text2video" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model_name": "kling-v1",
    "prompt": "宇航员站起身走了",
    "mode": "std",
    "duration": "5",
    "aspect_ratio": "16:9"
  }'
```

### 3.6 默认模型参考

以下是任务插件声明的默认模型列表；站点实际可用的模型由渠道配置决定。

| 插件 | 默认模型 |
|---|---|
| `alibaba` | `wan2.7-i2v`、`wan2.7-t2v`、`wan2.5-t2v-preview`、`wan2.5-i2v-preview`、`wan2.2-i2v-flash`、`wan2.2-i2v-plus`、`wanx2.1-i2v-plus`、`wanx2.1-i2v-turbo` |
| `doubao` | `doubao-seedance-1-0-pro-250528`、`doubao-seedance-1-0-lite-t2v`、`doubao-seedance-1-0-lite-i2v`、`doubao-seedance-1-5-pro-251215`、`doubao-seedance-2-0-260128`、`doubao-seedance-2-0-fast-260128`、`doubao-seedance-2-0-mini-260615`、`doubao-seedance-2-5-260628` |
| `google` | `veo-3.0-generate-001`、`veo-3.0-fast-generate-001`、`veo-3.1-generate-preview`、`veo-3.1-fast-generate-preview` |
| `hailuo` | `MiniMax-H3`、`MiniMax-Hailuo-2.3`、`MiniMax-Hailuo-2.3-Fast`、`MiniMax-Hailuo-02`、`T2V-01-Director`、`T2V-01`、`I2V-01-Director`、`I2V-01-live`、`I2V-01`、`S2V-01` |
| `jimeng` | `jimeng_vgfm_t2v_l20` |
| `kling` | `kling-v1`、`kling-v1-6`、`kling-v2-master` |
| `sora` | `sora-2`、`sora-2-pro` |
| `vertex-ai` | `veo-3.0-generate-001`、`veo-3.0-fast-generate-001`、`veo-3.1-generate-preview`、`veo-3.1-fast-generate-preview` |
| `vidu` | `viduq3-mix`、`viduq3-turbo`、`viduq3`、`viduq2-pro`、`viduq2`、`viduq1`、`vidu2.0`、`vidu1.5` |

图片模型没有统一的默认列表，取决于渠道类型与管理员配置：

- OpenAI：`dall-e-2`、`dall-e-3`、`gpt-image-1`、`gpt-image-1-mini`、`gpt-image-1.5`、`chatgpt-image-latest`
- Gemini/Vertex：Imagen 系列
- 阿里 DashScope：qwen-image 系列
- MiniMax：`image-01`、`image-01-live`
- Image Edit (Base64)：由管理员在渠道中声明模型

## 4. 计费与参数边界

- 图片 `n` 上限 **128**（`dto.MaxImageN`）
- 视频时长上限 **3600 秒**（`relaycommon.MaxTaskDurationSeconds`）
- 图片计费维度：模型倍率 + `n` + `size` + `quality`（dall-e 的 size/quality 有内置倍率）
- 视频计费维度通常包括：模型倍率/价格表、`seconds`、`resolution`、`size`、`mode`、`sound`、是否含视频输入等
- 视频提交会预扣，失败/异常会退款或按已产生用量结算
- 参数既影响上游生成，也影响计费；不要传上游不支持但计费表认识的取值，否则可能出现"按 A 档收费、上游按 B 档执行"的漂移

代码：`relay/common/relay_utils.go:182`、`relay/relay_task.go:221`、`relay/helper/video_price.go`。

## 5. 常见坑

1. **图片编辑渠道 63 只收 JSON + base64**：multipart、远程 URL、`images[]` 都会 400。
2. **视频统一入口的未知顶层字段会被丢弃**：`width`、`height`、`fps`、`seed`、`n` 要放进 `metadata`。
3. **统一视频入口 `prompt` 必填**：图生视频也要给；只有原生路由/插件路由可以只给图。
4. **`seconds` 在统一任务体里要写成字符串**（`"5"`）；`duration` 可数字或数字字符串。
5. **`metadata` 不是统一透传**：doubao/kling/vidu/google 多数整体合并，hailuo/jimeng 是白名单，alibaba 只读 `metadata.input` 和 `metadata.parameters`。
6. **`metadata.model` 不能改模型**：Go 适配器会删除该字段，alibaba 插件会直接报错。
7. **Ark `content[]` 只对 doubao/astraflow 开放**：其他渠道传 `content` 会 400。
8. **`/v1/videos/generations/tasks` 只对 doubao/astraflow 开放**：其他渠道任务在该端点查询也会被拒绝。
9. **图片模型能力靠标签声明**：Media Studio 只识别模型元数据里的 `text-to-image`、`image-to-image` 标签；`/v1/models` 的 `image-generation` endpoint 也来自 `text-to-image` 标签。
10. **`docs/openapi/relay.json` 的 `VideoRequest` 是旧示例**：实际统一任务体不读取其中的 `width`/`height`/`fps`。

## 6. 代码索引

| 主题 | 代码 |
|---|---|
| 图片请求 DTO | `relaykit/dto/openai_image.go:17` |
| 图片请求校验 | `relay/helper/valid_request.go:182` |
| 图片 relay 主流程 | `relay/image_handler.go:23` |
| OpenAI 图片转换/响应 | `relay/channel/openai/adaptor.go:523`、`relay/channel/openai/relay_image.go` |
| 图片编辑渠道 63 | `relay/channel/imageedit/adaptor.go:81` |
| 阿里图片方言 | `relay/channel/ali/image.go:24`、`relay/channel/ali/dto.go:173` |
| MiniMax 图片方言 | `relay/channel/minimax/image.go:42` |
| Gemini 图片方言 | `relay/channel/gemini/adaptor.go:66` |
| 统一任务体 | `relay/common/relay_info.go:1040` |
| 任务请求校验 | `relay/common/relay_utils.go:325` |
| 任务提交主流程 | `relay/relay_task.go:221` |
| 任务查询 | `relay/relay_task.go:474` |
| 视频内容代理 | `controller/video_proxy.go:63` |
| doubao 任务适配器 | `relay/channel/task/doubao/adaptor.go` |
| astraflow 任务适配器 | `relay/channel/task/astraflow/adaptor.go` |
| JS 任务插件适配器 | `relay/channel/task/jsplugin/adaptor.go` |
| Ark 任务响应 DTO | `dto/ark_video.go:20` |
| 视频路由 | `router/video-router.go`、`router/task-router.go` |
| 任务插件协议 | `docs/plugin-api/v1.md` |
| 各供应商任务插件 | `plugins/tasks/*/plugin.js` |
