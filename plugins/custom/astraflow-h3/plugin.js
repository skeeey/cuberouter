/*
Copyright (C) 2023-2026 QuantumNous
Copyright (C) 2026 CubeRouter

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export const meta = {
  apiVersion: 1,
  key: "astraflow-h3",
  name: "AstraFlow MiniMax H3",
  icon: "Minimax.Color",
  description: {
    en: "MiniMax-H3 video generation on AstraFlow (text-to-video, first/last-frame image-to-video, multimodal reference-to-video).",
    zh: "星图 MiniMax-H3 视频生成（文生视频、首尾帧图生视频、多模态参考生视频）。",
  },
  version: "1.0.1",
  author: { name: "Wei Liu" },
  models: ["MiniMax-H3"],
  fetchMode: "per_task",
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: {
        en: "Upstream billed seconds: output duration, or the total the upstream reports when reference-video input seconds are included in usage.duration.",
        zh: "上游计费秒数（输出时长；上游把参考视频输入秒数合并进 usage.duration 时按合并值计）。",
      },
    },
    resolution: {
      enum: ["768P", "2K"],
      description: {
        en: "Output resolution tier billed by upstream.",
        zh: "计费命中的输出分辨率档位。",
      },
    },
    input_images: {
      type: "number",
      unit: "count",
      description: {
        en: "Billed input image count: the first five input images are free, this is the excess.",
        zh: "计费输入图片数量（输入图片 5 张以内免费，这里是超出部分）。",
      },
    },
  },
  usageExamples: [
    { label: "768P · 5s", facts: { seconds: 5, resolution: "768P", input_images: 0 } },
    { label: "2K · 5s", facts: { seconds: 5, resolution: "2K", input_images: 0 } },
    { label: "2K · 5s · 7 images", facts: { seconds: 5, resolution: "2K", input_images: 2 } },
  ],
};

// 上游 = 星图（AstraFlow）统一任务接口：提交 POST /v1/tasks/submit，查询
// GET /v1/tasks/status?task_id=。请求体沿用 Ark 形态：
// {model, input:{content:[{type:text|image_url|video_url|audio_url, ...}]},
//  parameters:{resolution,duration,ratio?}}。
//
// MiniMax-H3 契约（2026-10-27 对上游实测）：resolution 必填且只接受 768P/2K；
// 文生视频 ratio 必填且不能 adaptive（可用 16:9/4:3/1:1/3:4/9:16/21:9）；
// 首/尾帧模式不下发 ratio；时长 4~15 秒；输入图片 ≤9 张，5 张以内免费。
// 插件在这里补齐缺省值并把不合法的值挡在预扣费之前，保证"发给上游的参数"与
// "用于计费的 usage facts"是同一份归一结果。
const SUBMIT_PATH = "/v1/tasks/submit";
const STATUS_PATH = "/v1/tasks/status";

const MIN_DURATION = 4;
const MAX_DURATION = 15;
const DEFAULT_DURATION = 5;
// 缺省走最便宜的 768P，避免客户端没写分辨率时被按 2K 档计费
const DEFAULT_RESOLUTION = "768P";
// 文生视频必填比例，缺省 16:9；参考场景缺省 adaptive（省略即可）
const DEFAULT_RATIO = "16:9";
const RATIOS = ["16:9", "4:3", "1:1", "3:4", "9:16", "21:9"];

const MEDIA_TYPES = ["image_url", "video_url", "audio_url"];
const ROLES = ["first_frame", "last_frame", "reference_image", "reference_video", "reference_audio"];
const FRAME_ROLES = ["first_frame", "last_frame"];
// 输入图片 5 张以内免费（上游 image_input>5 计价）
const FREE_INPUT_IMAGES = 5;
const MAX_IMAGES = 9;

function trimmed(value) {
  if (value === undefined || value === null) return "";
  return String(value).trim();
}

function asObject(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  return value;
}

// 时长取值与计费口径一致：顶层 duration > seconds，缺省 5 秒；越界直接拒绝
// （上游 4~15 秒，超出会在异步阶段失败，不如在预扣费之前挡下）。
function outboundDuration(request) {
  const candidates = [request && request.duration, request && request.seconds];
  for (const raw of candidates) {
    if (raw === undefined || raw === null || trimmed(raw) === "") continue;
    const seconds = Number(raw);
    if (!Number.isInteger(seconds) || seconds < MIN_DURATION || seconds > MAX_DURATION) {
      throw new Error("duration must be an integer between " + MIN_DURATION + " and " + MAX_DURATION + " seconds");
    }
    return seconds;
  }
  return DEFAULT_DURATION;
}

// 分辨率归一：字面量 768P/2K 直接命中；"720p"/"1080p" 这类档位与 "1344x768"
// 这类尺寸按短边归档（≥1440 → 2K，≥640 → 768P），比 640 更小或无法识别则
// 拒绝——绝不让"上游出的档位"与"计费的档位"出现分歧。
function outboundResolution(request) {
  const raw = trimmed(request && request.size) || trimmed(request && request.resolution);
  if (!raw) return DEFAULT_RESOLUTION;
  const normalized = raw.toUpperCase();
  if (normalized === "768P" || normalized === "2K") return normalized;
  const tier = normalized.match(/^(\d{3,4})P$/);
  if (tier) {
    const shortSide = Number(tier[1]);
    if (shortSide >= 1440) return "2K";
    if (shortSide >= 640) return "768P";
  }
  const size = normalized.match(/^(\d{2,5})\s*[*X×]\s*(\d{2,5})$/);
  if (size) {
    const shortSide = Math.min(Number(size[1]), Number(size[2]));
    if (shortSide >= 1440) return "2K";
    if (shortSide >= 640) return "768P";
  }
  throw new Error("unsupported resolution " + raw + "; MiniMax-H3 supports 768P or 2K");
}

// 素材来源：metadata.content 是完整直通口（Ark 内容项，参考视频/音频只能由此
// 传入）；未提供时按 image/images/input_reference 的图数推导：1 张 = 首帧，
// 2 张 = 首尾帧，3 张及以上 = 参考图（上游参考图最多 9 张）。multipart 上传
// 的文件（image / input_reference 字段）只在没有 URL 素材时使用。
function mediaSources(request, files) {
  const sources = [];
  const candidates = [trimmed(request && request.image)];
  if (Array.isArray(request && request.images)) {
    for (const image of request.images) candidates.push(trimmed(image));
  }
  candidates.push(trimmed(request && request.input_reference));
  for (const candidate of candidates) {
    if (candidate && sources.indexOf(candidate) < 0) sources.push(candidate);
  }
  if (sources.length === 0) {
    for (const file of files || []) {
      const field = trimmed(file && file.field);
      const ref = trimmed(file && file.ref);
      if (!ref || (field !== "image" && field !== "input_reference")) continue;
      sources.push({ __fileRef: ref, encoding: "dataUrl" });
    }
  }
  return sources;
}

function buildMedia(request, files) {
  const metadata = asObject(request && request.metadata) || {};
  const declared = metadata.content;
  if (declared !== undefined) {
    if (!Array.isArray(declared) || declared.length === 0) {
      throw new Error("metadata.content must be a non-empty array");
    }
    const items = [];
    for (const raw of declared) {
      const item = asObject(raw);
      const type = trimmed(item && item.type);
      const url = item && item.url;
      if (!item || MEDIA_TYPES.indexOf(type) < 0 || typeof url !== "string" || !trimmed(url)) {
        throw new Error("metadata.content entries must be {type, url, role?} with type one of " + MEDIA_TYPES.join(", "));
      }
      const role = trimmed(item.role);
      if (role && ROLES.indexOf(role) < 0) {
        throw new Error("unsupported content role " + role);
      }
      items.push({ type: type, url: trimmed(url), role: role });
    }
    assignDefaultRoles(items);
    validateMedia(items);
    return items;
  }

  const sources = mediaSources(request, files);
  if (sources.length > MAX_IMAGES) {
    throw new Error("too many input images (" + sources.length + ", max " + MAX_IMAGES + ")");
  }
  const items = [];
  for (let index = 0; index < sources.length; index++) {
    let role = "reference_image";
    if (sources.length === 1) role = "first_frame";
    else if (sources.length === 2 && index === 0) role = "first_frame";
    else if (sources.length === 2) role = "last_frame";
    items.push({ type: "image_url", url: sources[index], role: role });
  }
  validateMedia(items);
  return items;
}

// 无角色内容项按张数补角色（1 张首帧 / 2 张首尾帧 / 3 张及以上参考图），与
// 百炼 H3 插件一致；只要客户端显式给了任一图片角色就原样保留。含参考视频/
// 音频时无角色图片按参考图处理（不能与首尾帧模式混用）。
function assignDefaultRoles(items) {
  const images = items.filter(function (item) {
    return item.type === "image_url";
  });
  const hasNonImage = items.some(function (item) {
    return item.type !== "image_url";
  });
  const hasExplicit = images.some(function (item) {
    return item.role !== "";
  });
  if (!hasExplicit) {
    images.forEach(function (item, index) {
      if (hasNonImage) item.role = "reference_image";
      else if (images.length === 1) item.role = "first_frame";
      else if (images.length === 2 && index === 0) item.role = "first_frame";
      else if (images.length === 2) item.role = "last_frame";
      else item.role = "reference_image";
    });
  }
  for (const item of items) {
    if (item.type === "video_url" && !item.role) item.role = "reference_video";
    if (item.type === "audio_url" && !item.role) item.role = "reference_audio";
  }
}

function validateMedia(items) {
  let images = 0;
  let frames = 0;
  let references = 0;
  for (const item of items) {
    if (item.type === "image_url") {
      images++;
      if (item.role === "reference_image") references++;
      else frames++;
    } else {
      references++;
    }
  }
  if (images > MAX_IMAGES) {
    throw new Error("too many input images (" + images + ", max " + MAX_IMAGES + ")");
  }
  if (frames > 0 && references > 0) {
    throw new Error("first_frame/last_frame cannot be mixed with reference media");
  }
}

// 计费图片数量：上游 image_input>5 口径（超出 5 张免费额度的部分）
function billedInputImages(items) {
  let images = 0;
  for (const item of items) {
    if (item.type === "image_url") images++;
  }
  return Math.max(0, images - FREE_INPUT_IMAGES);
}

// 比例规则：首/尾帧场景宽高比由输入图片决定（上游恒 adaptive，不传）；
// 参考场景可选（缺省 adaptive）；文生视频必填且不能 adaptive。
function outboundRatio(request, hasFrames, hasReferences) {
  if (hasFrames) return "";
  const raw = trimmed(request && request.ratio);
  if (!raw) return hasReferences ? "" : DEFAULT_RATIO;
  if (raw === "adaptive") {
    if (!hasReferences) {
      throw new Error("ratio must not be adaptive for text-to-video; pick one of " + RATIOS.join(", "));
    }
    return "";
  }
  if (RATIOS.indexOf(raw) < 0) {
    throw new Error("ratio must be one of " + RATIOS.join(", "));
  }
  return raw;
}

// 请求归一结果：submit 请求体与 submit 期计费 facts 都从这一份推导，避免两处口径分叉
function plan(request, files) {
  const prompt = trimmed(request && request.prompt);
  if (!prompt) throw new Error("prompt is required");
  const media = buildMedia(request, files);
  const hasFrames = media.some(function (item) {
    return item.type === "image_url" && FRAME_ROLES.indexOf(item.role) >= 0;
  });
  const hasReferences = media.some(function (item) {
    return item.type !== "image_url" || item.role === "reference_image";
  });
  if (hasFrames && hasReferences) {
    throw new Error("first_frame/last_frame cannot be mixed with reference media");
  }
  return {
    prompt: prompt,
    media: media,
    duration: outboundDuration(request),
    resolution: outboundResolution(request),
    ratio: outboundRatio(request, hasFrames, hasReferences),
    inputImages: billedInputImages(media),
    action: media.length === 0 ? "text_to_video" : hasFrames ? "image_to_video" : "reference_to_video",
  };
}

function contentItems(media) {
  const content = [];
  for (const item of media) {
    const entry = { type: item.type };
    if (item.type === "image_url") entry.image_url = { url: item.url };
    else if (item.type === "video_url") entry.video_url = { url: item.url };
    else entry.audio_url = { url: item.url };
    if (item.role) entry.role = item.role;
    content.push(entry);
  }
  return content;
}

export function buildSubmitRequest(ctx) {
  const request = ctx.requestBody || {};
  const model = trimmed(ctx.upstreamModel) || trimmed(request.model);
  if (!model) throw new Error("model is required");
  const planned = plan(request, ctx.files);
  const parameters = { resolution: planned.resolution, duration: planned.duration };
  if (planned.ratio) parameters.ratio = planned.ratio;
  const metadata = asObject(request.metadata) || {};
  if (metadata.watermark === true) parameters.watermark = true;
  const content = contentItems(planned.media);
  content.push({ type: "text", text: planned.prompt });
  return {
    url: ctx.baseUrl + SUBMIT_PATH,
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
      Authorization: "Bearer " + ctx.apiKey,
    },
    body: { model: model, input: { content: content }, parameters: parameters },
    action: planned.action,
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = asObject(resp && resp.body);
  if (!body) throw new Error("unexpected submit response");
  const output = asObject(body.output) || {};
  const taskId = trimmed(output.task_id);
  if (taskId) return { taskId: taskId, taskData: body };
  const error = asObject(body.error) || {};
  const code = trimmed(error.code) || trimmed(body.code);
  const message = trimmed(error.message) || trimmed(body.message);
  const detail = [code, message].filter(function (part) {
    return part !== "";
  });
  throw new Error(detail.length ? detail.join(": ") : "submit response has no task id");
}

export function buildQueryRequest(ctx) {
  return {
    url: ctx.baseUrl + STATUS_PATH + "?task_id=" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
  };
}

// 上游任务状态：Pending / Running / Success / Failure / Expired
export function parseTaskResult(ctx, body) {
  const envelope = asObject(body) || {};
  const output = asObject(envelope.output) || {};
  const status = trimmed(output.task_status).toLowerCase();
  if (status === "pending") return { status: "QUEUED" };
  if (status === "running") return { status: "IN_PROGRESS" };
  if (status === "success") {
    const urls = Array.isArray(output.urls) ? output.urls : [];
    const url = trimmed(urls[0]);
    if (!url) return { status: "FAILURE", reason: "task succeeded without a video url" };
    return { status: "SUCCESS", url: url };
  }
  if (status === "failure" || status === "expired") {
    const message = trimmed(output.error_message);
    return { status: "FAILURE", reason: message || "task " + status };
  }
  // 未知状态收敛为 UNKNOWN：轮询层按失败预算处理，避免无限等待
  return { status: "UNKNOWN", reason: "unrecognized task status: " + trimmed(output.task_status) };
}

export function extractUsage(ctx) {
  // 计费走模型级 tiered_expr（网关按插件 usage facts 求值），不提供 billing_ratios 系数
  if (ctx.usagePurpose === "billing_ratios") return null;
  const request = ctx.requestBody || {};
  const planned = plan(request, ctx.files);
  return {
    seconds: planned.duration,
    resolution: planned.resolution,
    input_images: planned.inputImages,
  };
}

// 完成态 facts：秒数取上游 usage.duration（上游在参考视频场景可能合并输入
// 秒数；缺失时依次退回 output_video_duration / total_seconds / input+output）。
// input_images 不覆盖——上游查询响应不回报图片数，提交时的估算值即最终口径。
export function extractUsageOnComplete(task, taskResult, body) {
  const envelope = asObject(body) || {};
  const usage = asObject(envelope.usage) || {};
  const facts = {};
  const seconds = completionSeconds(usage);
  if (seconds !== null && seconds > 0) facts.seconds = Math.min(seconds, 3600);
  return facts;
}

function completionSeconds(usage) {
  const direct = usageNumber(usage.duration);
  if (direct !== null && direct > 0) return direct;
  const output = usageNumber(usage.output_video_duration);
  if (output !== null && output > 0) return output;
  const total = usageNumber(usage.total_seconds);
  if (total !== null && total > 0) return total;
  const input = usageNumber(usage.input_seconds);
  const outputSeconds = usageNumber(usage.output_seconds);
  if (input !== null && outputSeconds !== null) return input + outputSeconds;
  return null;
}

function usageNumber(value) {
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) return null;
  return value;
}

// 查询响应可能是上游直出 {output:{...}}，也可能是平台包装 {code, data:{output}}；
// 结果地址与状态都取到同一层字段。
function taskOutput(value) {
  const body = asObject(value) || {};
  const inner = asObject(body.data) || body;
  return asObject(inner.output) || inner;
}

function taskVideoURL(data) {
  const output = taskOutput(data);
  const urls = Array.isArray(output.urls) ? output.urls : [];
  return trimmed(urls[0]);
}

export function listArtifacts(task) {
  if (!task || task.status !== "SUCCESS") return [];
  return taskVideoURL(task.data) ? [{ key: "video", type: "video" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const url = taskVideoURL(ctx.data);
  if (!url) throw new Error("artifact_not_found");
  // 结果地址是 OSS 临时签名 URL（鉴权在查询串里），不带渠道凭证直接回源
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}
