export const meta = {
  apiVersion: 1,
  key: "alibaba",
  name: "Alibaba Bailian",
  icon: "Bailian.Color",
  description: {
    en: "Alibaba Cloud Bailian Wanxiang and Yike/WonderClip video generation (text-to-video, image-to-video, first/last frame, reference-to-video)",
    zh: "阿里云百炼万相 / 万镜一刻视频生成（文生视频、图生视频、首尾帧、参考生视频）",
  },
  version: "1.1.0",
  author: { name: "QuantumNous" },
  channelTypes: [17],
  models: [
    "wan2.7-i2v",
    "wan2.7-t2v",
    "wan2.5-t2v-preview",
    "wan2.5-i2v-preview",
    "wan2.2-i2v-flash",
    "wan2.2-i2v-plus",
    "wanx2.1-i2v-plus",
    "wanx2.1-i2v-turbo",
  ],
  fetchMode: "per_task",
  usageSchema: {
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Requested video duration in seconds.", zh: "请求的视频时长，单位为秒。" },
    },
    resolution: {
      enum: ["480P", "720P", "1080P"],
      description: { en: "Requested output video resolution.", zh: "请求的输出视频分辨率。" },
    },
  },
  routes: [
    { method: "POST", path: "/ali/api/v1/services/aigc/video-generation/video-synthesis", type: "submit", decode: "createVideoTask", render: "taskCreated" },
    { method: "GET", path: "/ali/api/v1/tasks/:task_id", type: "query", render: "taskStatus" },
  ],
  protocols: [{ name: "openai_responses", supports: ["stream", "sync", "background"] }, "openai_video"],
};

function trimmed(value) {
  return String(value || "").trim();
}

function firstImage(req) {
  if (trimmed(req.image)) return trimmed(req.image);
  for (const image of req.images || []) if (trimmed(image)) return trimmed(image);
  return trimmed(req.input_reference);
}

function secondImage(req) {
  let count = 0;
  for (const image of req.images || []) {
    if (!trimmed(image)) continue;
    count++;
    if (count === 2) return trimmed(image);
  }
  return "";
}

function normalizeResolution(value) {
  let resolution = String(value || "").toUpperCase();
  if (!resolution.endsWith("P")) resolution += "P";
  return resolution;
}

// 万镜一刻（Yike）是与 DashScope 完全不同的一套 RPC：所有业务参数都在 query
// 中，POST 根路径 /，body 为空；提交返回 JobId，必须轮询 GetVideoGenerationJob。
// 该上游仍挂在 Ali（17 类）渠道下，按 base_url 主机名分流。
const yikeAPIVersion = "2026-07-07";
const yikeSubmitAction = "SubmitVideoGenerationJob";
const yikeGetAction = "GetVideoGenerationJob";

function isYikeBaseURL(baseURL) {
  const match = /^(?:https?):\/\/([^\/?#]+)/i.exec(String(baseURL || "").trim());
  if (!match) return false;
  const host = match[1].toLowerCase();
  return host.indexOf("yike.") === 0 && host.slice(-".aliyuncs.com".length) === ".aliyuncs.com";
}

function yikeRootURL(baseURL) {
  const match = /^(https?):\/\/([^\/?#]+)/i.exec(String(baseURL || "").trim());
  if (!match || match[2].indexOf("yike.") !== 0) throw new Error("invalid Yike base URL");
  return match[1].toLowerCase() + "://" + match[2];
}

function yikeTimestamp() {
  return new Date().toISOString().replace(/\.\d{3}Z$/, "Z");
}

function yikeQueryString(params) {
  const parts = [];
  for (const key of Object.keys(params)) {
    const value = params[key];
    if (value === undefined || value === null || value === "") continue;
    parts.push(encodeURIComponent(key) + "=" + encodeURIComponent(String(value)));
  }
  return parts.join("&");
}

// yikeOutputSize 把 OpenAI 风格的 size / resolution / ratio 归一为 Yike 的
// Resolution + AspectRatio。只接受价格表覆盖的 480P/720P/1080P，避免请求落到
// 上游支持但网关未配置的档位（会按锚点价计费）。
function yikeOutputSize(req, metadata) {
  let resolution = "";
  let aspectRatio = "";
  const size = String(req.size || "").trim();
  if (size) {
    const upper = size.toUpperCase();
    if (upper === "480P" || upper === "720P" || upper === "1080P") {
      resolution = upper;
    } else {
      const normalized = size.toLowerCase().replace(/[×*]/g, "x");
      const parts = normalized.split("x");
      if (parts.length !== 2) throw new Error("unsupported Yike size: " + size + ", example: 1280x720");
      const width = Number(parts[0]);
      const height = Number(parts[1]);
      if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0) {
        throw new Error("unsupported Yike size: " + size + ", example: 1280x720");
      }
      // 必须与 relay/helper.VideoResolutionTier 的分档一致：计费按请求的 size
      // 归一档位，上游档位若与计费档位错开（如 960x540 计费是 540p）会按锚点
      // 价收费。540p/360p/4k 没有价格行，下面统一拒绝。
      const longest = Math.max(width, height);
      if (longest >= 3840) resolution = "4k";
      else if (longest >= 1920) resolution = "1080P";
      else if (longest >= 1280) resolution = "720P";
      else if (longest >= 960) resolution = "540P";
      else if (longest >= 640) resolution = "480P";
      else resolution = "360P";
      const sizeAspects = {
        "832x480": "16:9", "854x480": "16:9", "640x360": "16:9",
        "480x832": "9:16", "480x854": "9:16", "360x640": "9:16",
        "640x480": "4:3", "480x640": "3:4", "480x480": "1:1",
        "1280x720": "16:9", "720x1280": "9:16", "960x720": "4:3", "720x960": "3:4", "720x720": "1:1",
        "1920x1080": "16:9", "1080x1920": "9:16", "1440x1080": "4:3", "1080x1440": "3:4", "1080x1080": "1:1",
      };
      if (sizeAspects[normalized]) {
        aspectRatio = sizeAspects[normalized];
      } else {
        const ratio = width / height;
        if (Math.abs(ratio - 1) < 0.01) aspectRatio = "1:1";
        else if (Math.abs(ratio - 16 / 9) < 0.02) aspectRatio = "16:9";
        else if (Math.abs(ratio - 9 / 16) < 0.02) aspectRatio = "9:16";
        else if (Math.abs(ratio - 4 / 3) < 0.02) aspectRatio = "4:3";
        else if (Math.abs(ratio - 3 / 4) < 0.02) aspectRatio = "3:4";
        else throw new Error("unsupported Yike aspect ratio for size: " + size);
      }
    }
  }
  const resolutionOverride = String(req.resolution || "").trim();
  if (resolutionOverride) resolution = resolutionOverride.toUpperCase();
  const aspectOverride = String(req.ratio || (metadata && metadata.aspect_ratio) || "").trim();
  if (aspectOverride) aspectRatio = aspectOverride;
  if (!resolution) resolution = "720P";
  if (!aspectRatio) aspectRatio = "16:9";
  if (resolution !== "480P" && resolution !== "720P" && resolution !== "1080P") {
    throw new Error("unsupported Yike resolution: " + resolution + ", supported: 480P/720P/1080P");
  }
  if (["16:9", "9:16", "4:3", "3:4", "1:1", "adaptive"].indexOf(aspectRatio) < 0) {
    throw new Error("unsupported Yike aspect_ratio: " + aspectRatio);
  }
  return { resolution: resolution, aspectRatio: aspectRatio };
}

function yikeDuration(req) {
  let duration = Number(req.duration);
  if (!(duration > 0) && req.seconds !== undefined) duration = Number(req.seconds);
  if (!(duration > 0)) duration = 5;
  if (!Number.isInteger(duration)) throw new Error("invalid Yike duration: " + String(duration));
  // 上游对 wan3.0 系列的实测校验文案为 2–30 秒（文档旧值是 4–15）。
  if (duration < 2 || duration > 30) throw new Error("Yike duration must be between 2 and 30 seconds");
  return duration;
}

function yikeMediaUrl(value) {
  const url = trimmed(value);
  if (!url) throw new Error("Yike media url is required");
  const match = /^(https?):\/\/([^\/?#]+)/i.exec(url);
  if (!match) throw new Error("Yike media URL must be a public HTTP(S) URL");
  const host = match[2].toLowerCase();
  if (host === "localhost" || host.slice(-".localhost".length) === ".localhost" ||
      host.slice(-".local".length) === ".local" || host.slice(-".internal".length) === ".internal") {
    throw new Error("Yike media URL must be a public HTTP(S) URL");
  }
  return url;
}

function yikeMedia(input) {
  const media = input || {};
  let type = String(media.type || "").trim().toLowerCase();
  if (!type) type = "image";
  if (type !== "image" && type !== "video" && type !== "audio") {
    throw new Error("unsupported Yike media type: " + type);
  }
  const url = trimmed(media.url);
  const mediaId = trimmed(media.media_id);
  if ((url === "") === (mediaId === "")) {
    throw new Error("each Yike media requires exactly one of url or media_id");
  }
  const converted = { Type: type };
  if (url) converted.Url = yikeMediaUrl(url);
  if (mediaId) converted.MediaId = mediaId;
  return converted;
}

function yikeMedias(req, metadata) {
  const configured = metadata && metadata.medias;
  if (configured !== undefined && configured !== null) {
    if (!Array.isArray(configured)) throw new Error("metadata.medias must be an array");
    return configured.map(yikeMedia);
  }
  const images = [];
  const requestImages = Array.isArray(req.images) ? req.images : [];
  for (const image of requestImages) if (trimmed(image)) images.push(yikeMediaUrl(image));
  if (images.length === 0 && trimmed(req.image)) images.push(yikeMediaUrl(req.image));
  if (images.length === 0 && trimmed(req.input_reference)) images.push(yikeMediaUrl(req.input_reference));
  return images.map(function (url) {
    return { Type: "image", Url: url };
  });
}

function yikeJobType(req, metadata, medias) {
  let jobType = String((metadata && metadata.job_type) || "").trim();
  if (!jobType) {
    if (medias.length === 0) jobType = "text_to_video";
    else if (medias.length === 1 && medias[0].Type === "image") jobType = "image_to_video";
    else throw new Error("multiple media inputs require metadata.job_type");
  }
  if (jobType === "text_to_video") {
    if (medias.length !== 0) throw new Error("text_to_video does not accept media inputs");
  } else if (jobType === "image_to_video") {
    if (medias.length !== 1 || medias[0].Type !== "image") throw new Error("image_to_video requires exactly one image");
  } else if (jobType === "first_last_frame") {
    if (medias.length !== 2 || medias[0].Type !== "image" || medias[1].Type !== "image") {
      throw new Error("first_last_frame requires exactly two images");
    }
  } else if (jobType === "reference_to_video") {
    if (medias.length === 0) throw new Error("reference_to_video requires at least one media input");
  } else {
    throw new Error("unsupported Yike job_type: " + jobType);
  }
  return jobType;
}

// yikeOutputUrl 从轮询响应（或任务 data）中解析 Finished 任务的视频地址。
// Output 是 JSON 字符串，形如 {"Medias":[{"OutputUrl":"..."}]}。
function yikeOutputUrl(data) {
  const job = data && data.VideoGenerationJob;
  if (!job) return "";
  let output;
  try {
    output = typeof job.Output === "string" ? JSON.parse(job.Output) : job.Output;
  } catch (e) {
    return "";
  }
  const medias = (output && output.Medias) || [];
  return medias.length && medias[0] && medias[0].OutputUrl ? String(medias[0].OutputUrl) : "";
}

function buildYikeSubmitRequest(ctx) {
  const req = ctx.requestBody || {};
  const metadata = req.metadata || {};
  const apiKey = String(ctx.apiKey || "").trim();
  if (!apiKey) throw new Error("Yike API key is required");
  const model = String(ctx.upstreamModel || req.model || "").trim();
  if (!model) throw new Error("Yike model is required");
  const prompt = trimmed(req.prompt);
  if (!prompt) throw new Error("Yike prompt is required");
  if (metadata.n !== undefined && Number(metadata.n) !== 1) throw new Error("Yike only supports n=1");
  if (metadata.resolution !== undefined || metadata.duration !== undefined) {
    // 按秒计费只看顶层 size/duration；metadata 覆盖不会被计费读取，放宽会少收。
    throw new Error("Yike does not support metadata.resolution/metadata.duration; use top-level size/duration");
  }

  const medias = yikeMedias(req, metadata);
  const jobType = yikeJobType(req, metadata, medias);
  const size = yikeOutputSize(req, metadata);
  const duration = yikeDuration(req);
  const input = { Prompt: prompt };
  if (medias.length) input.Medias = medias;
  const query = {
    Action: yikeSubmitAction,
    Version: yikeAPIVersion,
    Timestamp: yikeTimestamp(),
    Format: "json",
    JobType: jobType,
    Model: model,
    Scene: "general",
    Input: JSON.stringify(input),
    Resolution: size.resolution,
    AspectRatio: size.aspectRatio,
    Duration: duration,
    N: 1,
  };
  if (ctx.publicTaskId) query.ClientToken = String(ctx.publicTaskId);
  return {
    url: yikeRootURL(ctx.baseUrl) + "/?" + yikeQueryString(query),
    method: "POST",
    headers: {
      Authorization: "Bearer " + apiKey,
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    action: jobType,
  };
}

function convert(ctx) {
  const req = ctx.requestBody;
  const upstreamModel = ctx.upstreamModel || req.model;
  const input = { prompt: req.prompt || "" };
  const image = firstImage(req);
  if (image) input.img_url = image;
  const parameters = { prompt_extend: true, duration: 5 };

  if (req.size) {
    if (String(req.model).includes("t2v") && !String(req.size).includes("*")) throw new Error("invalid size: " + req.size + ", example: 1920*1080");
    if (String(req.size).includes("*")) parameters.size = req.size;
    else parameters.resolution = normalizeResolution(req.size);
  } else if (String(req.model).includes("t2v")) {
    parameters.size = String(req.model).startsWith("wan2.5") || String(req.model).startsWith("wan2.2") ? "1920*1080" : "1280*720";
  } else if (String(req.model).startsWith("wan2.6") || String(req.model).startsWith("wan2.5") || String(req.model).startsWith("wan2.2-i2v-plus")) {
    parameters.resolution = "1080P";
  } else {
    parameters.resolution = "720P";
  }

  if (Number(req.duration) > 0) parameters.duration = Number(req.duration);
  else if (req.seconds) {
    const seconds = Number(req.seconds);
    if (!Number.isInteger(seconds)) throw new Error("convert seconds to int failed");
    parameters.duration = seconds > 0 ? seconds : 5;
  }

  const metadata = req.metadata || {};
  Object.assign(input, metadata.input || {});
  Object.assign(parameters, metadata.parameters || {});
  const model = metadata.model === undefined ? upstreamModel : metadata.model;
  if (model !== upstreamModel) throw new Error("can't change model with metadata");
  const body = { model: model, input: input, parameters: parameters };

  if (String(model).startsWith("wan2.7-i2v")) {
    if (!Array.isArray(input.media) || input.media.length === 0) {
      input.media = [];
      const first = trimmed(input.first_frame_url) || trimmed(input.img_url) || firstImage(req);
      const last = trimmed(input.last_frame_url) || secondImage(req);
      if (first) input.media.push({ type: "first_frame", url: first });
      if (last) input.media.push({ type: "last_frame", url: last });
      if (trimmed(input.audio_url)) input.media.push({ type: "driving_audio", url: input.audio_url });
    }
    if (input.media.length === 0) throw new Error("wan2.7-i2v requires image, images, input_reference, or input.media");
    delete input.img_url;
    delete input.first_frame_url;
    delete input.last_frame_url;
    delete input.audio_url;
  }
  if (!parameters.prompt_extend) delete parameters.prompt_extend;
  if (!parameters.watermark) delete parameters.watermark;
  if (!parameters.seed) delete parameters.seed;
  for (const key of ["resolution", "size"]) if (!parameters[key]) delete parameters[key];
  return body;
}

function resolutionRatio(body) {
  let resolution = body.parameters.size
    ? {
        "832*480": "480P",
        "480*832": "480P",
        "624*624": "480P",
        "1280*720": "720P",
        "720*1280": "720P",
        "960*960": "720P",
        "1088*832": "720P",
        "832*1088": "720P",
        "1920*1080": "1080P",
        "1080*1920": "1080P",
        "1440*1440": "1080P",
        "1632*1248": "1080P",
        "1248*1632": "1080P",
      }[body.parameters.size]
    : normalizeResolution(body.parameters.resolution);
  const ratios = {
    "wan2.6-i2v": { "720P": 1, "1080P": 1 / 0.6 },
    "wan2.5-t2v-preview": { "480P": 1, "720P": 2, "1080P": 1 / 0.3 },
    "wan2.2-t2v-plus": { "480P": 1, "1080P": 5 },
    "wan2.5-i2v-preview": { "480P": 1, "720P": 2, "1080P": 1 / 0.3 },
    "wan2.2-i2v-plus": { "480P": 1, "1080P": 5 },
    "wan2.2-kf2v-flash": { "480P": 1, "720P": 2, "1080P": 4.8 },
    "wan2.2-i2v-flash": { "480P": 1, "720P": 2 },
    "wan2.2-s2v": { "480P": 1, "720P": 1.8 },
  };
  return ratios[body.model] ? { key: "resolution-" + resolution, value: ratios[body.model][resolution] } : null;
}

function responsesInput(req) {
  const texts = [],
    images = [];
  const input = req.input;
  if (typeof input === "string") texts.push(input);
  else if (Array.isArray(input)) {
    for (const item of input) {
      if (typeof item === "string") {
        texts.push(item);
        continue;
      }
      if (!item || typeof item !== "object" || Array.isArray(item)) continue;
      const content = item.content === undefined ? [item] : Array.isArray(item.content) ? item.content : [item.content];
      for (const part of content) {
        if (typeof part === "string") {
          texts.push(part);
          continue;
        }
        if (!part || typeof part !== "object" || Array.isArray(part)) continue;
        if (["input_text", "text"].includes(part.type) && typeof part.text === "string") texts.push(part.text);
        if (["input_image", "image_url"].includes(part.type)) {
          let image = part.image_url;
          if (image && typeof image === "object") image = image.url;
          if (trimmed(image)) images.push(trimmed(image));
        }
      }
    }
  }
  return {
    prompt: texts
      .filter(function (text) {
        return trimmed(text);
      })
      .join("\n"),
    images: images,
  };
}

function responsesVideoText(ctx) {
  const artifact = ctx && ctx.artifacts && ctx.artifacts.video;
  const url = trimmed(artifact && artifact.url);
  if (!url) throw new Error("video artifact is unavailable");
  const escaped = url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + escaped + '"></video>';
}

export function buildSubmitRequest(ctx) {
  if (isYikeBaseURL(ctx.baseUrl)) return buildYikeSubmitRequest(ctx);
  const body = convert(ctx);
  return {
    url: ctx.baseUrl + "/api/v1/services/aigc/video-generation/video-synthesis",
    method: "POST",
    headers: { Authorization: "Bearer " + ctx.apiKey, "Content-Type": "application/json", "X-DashScope-Async": "enable" },
    body: body,
    action: firstImage(ctx.requestBody) ? "image_to_video" : "text_to_video",
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = resp.body || {};
  if (isYikeBaseURL(ctx.baseUrl)) {
    if (body.Code) throw new Error(body.Code + ": " + (body.Message || ""));
    if (!body.JobId) {
      throw new Error(body.Message ? "Yike error: " + body.Message : "Yike response did not contain JobId");
    }
    return { taskId: body.JobId, taskData: body };
  }
  if (body.code) throw new Error(body.code + ": " + (body.message || ""));
  if (!body.output || !body.output.task_id) throw new Error("task_id is empty");
  return { taskId: body.output.task_id, taskData: body };
}

export function extractUsage(ctx) {
  if (isYikeBaseURL(ctx.baseUrl)) {
    const req = ctx.requestBody || {};
    const metadata = req.metadata || {};
    const size = yikeOutputSize(req, metadata);
    return { seconds: yikeDuration(req), resolution: size.resolution };
  }
  const body = convert(ctx);
  if (ctx.usagePurpose === "billing_ratios") {
    const ratios = { seconds: Math.min(Number(body.parameters.duration), 3600) };
    const resolution = resolutionRatio(body);
    if (resolution && resolution.value !== undefined) ratios[resolution.key] = resolution.value;
    return ratios;
  }
  let resolution = body.parameters.size
    ? {
        "832*480": "480P",
        "480*832": "480P",
        "624*624": "480P",
        "1280*720": "720P",
        "720*1280": "720P",
        "960*960": "720P",
        "1088*832": "720P",
        "832*1088": "720P",
        "1920*1080": "1080P",
        "1080*1920": "1080P",
        "1440*1440": "1080P",
        "1632*1248": "1080P",
        "1248*1632": "1080P",
      }[body.parameters.size]
    : normalizeResolution(body.parameters.resolution);
  if (!["480P", "720P", "1080P"].includes(resolution)) resolution = "720P";
  return { seconds: Math.min(Number(body.parameters.duration), 3600), resolution: resolution };
}

export function extractUsageOnComplete(task, taskResult, body) {
  const job = body && body.VideoGenerationJob;
  if (job) {
    const facts = {};
    const seconds = Number(job.Duration || 0);
    if (Number.isFinite(seconds) && seconds > 0) facts.seconds = Math.min(seconds, 3600);
    const resolution = normalizeResolution(job.Resolution || "");
    if (["480P", "720P", "1080P"].includes(resolution)) facts.resolution = resolution;
    return facts;
  }
  const output = (body && body.output) || {};
  const facts = {};
  const seconds = Number(output.duration || output.duration_seconds || 0);
  if (Number.isFinite(seconds) && seconds > 0) facts.seconds = Math.min(seconds, 3600);
  const resolution = normalizeResolution(output.resolution || "");
  if (["480P", "720P", "1080P"].includes(resolution)) facts.resolution = resolution;
  return facts;
}

export function buildQueryRequest(ctx) {
  if (isYikeBaseURL(ctx.baseUrl)) {
    const apiKey = String(ctx.apiKey || "").trim();
    if (!apiKey) throw new Error("Yike API key is required");
    const query = {
      Action: yikeGetAction,
      Version: yikeAPIVersion,
      Timestamp: yikeTimestamp(),
      Format: "json",
      JobId: ctx.taskId,
    };
    return {
      url: yikeRootURL(ctx.baseUrl) + "/?" + yikeQueryString(query),
      method: "GET",
      headers: { Authorization: "Bearer " + apiKey, Accept: "application/json" },
    };
  }
  return { url: ctx.baseUrl + "/api/v1/tasks/" + ctx.taskId, method: "GET", headers: { Authorization: "Bearer " + ctx.apiKey } };
}

export function parseTaskResult(ctx, body) {
  if (isYikeBaseURL(ctx && ctx.baseUrl)) {
    const job = body && body.VideoGenerationJob;
    if (!job) {
      if (body && (body.Code || body.Message)) {
        return { status: "FAILURE", reason: String(body.Code || "YikeError") + ": " + String(body.Message || "") };
      }
      // 空响应/瞬时错误保持 UNKNOWN，让轮询层按失败重试而不是直接终止任务。
      return { status: "UNKNOWN", reason: "Yike response did not contain VideoGenerationJob" };
    }
    const status = String(job.Status || "");
    if (status === "Created" || status === "Queuing") return { status: "QUEUED" };
    if (status === "Executing") return { status: "IN_PROGRESS" };
    if (status === "Finished") {
      let output;
      try {
        output = typeof job.Output === "string" ? JSON.parse(job.Output) : job.Output;
      } catch (e) {
        return { status: "FAILURE", reason: "invalid Yike output: " + e.message };
      }
      const medias = (output && output.Medias) || [];
      const url = medias.length && medias[0] && medias[0].OutputUrl ? String(medias[0].OutputUrl) : "";
      if (!url) return { status: "FAILURE", reason: "Yike finished task did not contain OutputUrl" };
      return { status: "SUCCESS", url: url };
    }
    if (status === "Failed") {
      return { status: "FAILURE", reason: String(job.ErrorMessage || body.Message || "task failed") };
    }
    return { status: "UNKNOWN", reason: "unrecognized Yike status: " + status };
  }
  const output = body.output || {};
  if (output.task_status === "PENDING") return { status: "QUEUED" };
  if (output.task_status === "RUNNING") return { status: "IN_PROGRESS" };
  if (output.task_status === "SUCCEEDED") return { status: "SUCCESS", url: output.video_url || "" };
  if (["FAILED", "CANCELED", "UNKNOWN"].includes(output.task_status)) {
    let reason = body.message || "";
    if (!reason && output.message) reason = "task failed, code: " + (output.code || "") + " , message: " + output.message;
    if (!reason) reason = "task failed";
    return { status: "FAILURE", reason: reason };
  }
  return { status: "UNKNOWN", reason: "unrecognized status: " + String(output.task_status || "") };
}

function artifactData(ctx) {
  const data = (ctx && ctx.data) || {};
  if (data.data && typeof data.data === "object" && data.data.task_id && Object.prototype.hasOwnProperty.call(data.data, "data")) return data.data.data || {};
  return data;
}

export function listArtifacts(task) {
  const data = artifactData(task);
  if (task.status === "SUCCESS" && trimmed(yikeOutputUrl(data))) return [{ key: "video", type: "video" }];
  const output = data.output || {};
  return task.status === "SUCCESS" && trimmed(output.video_url) ? [{ key: "video", type: "video" }] : [];
}

export function buildContentRequest(ctx) {
  if (ctx.artifactKey !== "video") throw new Error("artifact_not_found");
  const data = artifactData(ctx);
  const yikeURL = trimmed(yikeOutputUrl(data));
  if (yikeURL) return { url: yikeURL, method: ctx.clientRequest.method, credentialless: true };
  const url = trimmed((data.output || {}).video_url);
  if (!url) throw new Error("artifact_not_found");
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}

export const native = {
  createVideoTask: function (ctx) {
    if (!ctx.body || ctx.body.kind !== "json" || !ctx.body.value || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
    const req = ctx.body.value,
      input = req.input || {},
      parameters = req.parameters || {};
    return {
      kind: "submit",
      model: req.model,
      action: input.img_url ? "image_to_video" : "text_to_video",
      requestBody: {
        model: req.model,
        prompt: input.prompt || "",
        image: input.img_url,
        duration: parameters.duration,
        size: parameters.size || parameters.resolution,
      },
    };
  },
  taskCreated: function (ctx, task) {
    const data = task.data || {};
    return { request_id: data.request_id || "", output: { task_id: task.task_id, task_status: "PENDING" } };
  },
  taskStatus: function (ctx, task) {
    const data = task.data || {},
      output = Object.assign({}, data.output || {}, { task_id: task.task_id });
    return Object.assign({}, data, { output: output });
  },
  error: function (ctx, error) {
    return { code: error.code, message: error.message, request_id: "" };
  },
};

export const protocols = {
  openai_responses: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
      const req = ctx.body.value;
      if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
      const model = trimmed(ctx.model);
      if (!model) throw new Error("model is required");
      if (req.input !== undefined && typeof req.input !== "string" && !Array.isArray(req.input)) throw new Error("input must be a string or array");
      const input = responsesInput(req);
      const prompt = input.prompt || trimmed(req.prompt);
      const requestBody = { model: model, prompt: prompt };
      if (trimmed(req.image)) requestBody.image = trimmed(req.image);
      if (req.images !== undefined && !Array.isArray(req.images)) throw new Error("images must be an array");
      const images = [];
      for (const image of req.images || []) if (trimmed(image) && !images.includes(trimmed(image))) images.push(trimmed(image));
      for (const image of input.images) if (!images.includes(image)) images.push(image);
      if (images.length) requestBody.images = images;
      if (trimmed(req.input_reference)) requestBody.input_reference = trimmed(req.input_reference);
      for (const key of ["size", "duration", "seconds"]) {
        if (Object.prototype.hasOwnProperty.call(req, key)) requestBody[key] = req[key];
      }
      if (Object.prototype.hasOwnProperty.call(req, "metadata")) requestBody.metadata = req.metadata;
      if (!prompt && (!model.includes("i2v") || !firstImage(requestBody))) throw new Error("input is required");
      return { kind: "submit", model: model, action: firstImage(requestBody) ? "image_to_video" : "text_to_video", requestBody: requestBody };
    },
    renderEvents: function (ctx, task, previousState) {
      const status = String(task.status || "UNKNOWN").toUpperCase();
      const value = Number(String(task.progress || "").replace("%", ""));
      const progress = Number.isFinite(value) && value >= 0 && value <= 100 ? value : null;
      const state = { status: status, progress: progress };
      if (status === "SUCCESS") {
        const text = responsesVideoText(ctx);
        const events = previousState && previousState.status === status ? [] : text ? [{ type: "output", data: text }] : [];
        return { events: events, state: state, done: true };
      }
      if (status === "FAILURE") {
        return { events: [{ type: "error", code: "task_failed", message: "task failed" }], state: state, done: true };
      }
      if (previousState && previousState.status === status && previousState.progress === progress) {
        return { events: [], state: state, done: false };
      }
      const event = { type: "progress", message: status.toLowerCase() };
      if (progress !== null) event.progress = progress;
      return { events: [event], state: state, done: false };
    },
    renderFinal: function (ctx, _task) {
      return {
        output: [
          {
            type: "message",
            status: "completed",
            role: "assistant",
            content: [
              {
                type: "output_text",
                text: responsesVideoText(ctx),
                annotations: [],
                logprobs: [],
              },
            ],
          },
        ],
        metadata: { vendor: "ali" },
      };
    },
  },
  openai_video: {
    decodeRequest: function (ctx) {
      let req;
      if (ctx.body && ctx.body.kind === "json") req = ctx.body.value;
      else if (ctx.body && ctx.body.kind === "multipart") {
        if ((ctx.body.files || []).length) throw new Error("Alibaba requires image references to be URLs");
        const first = function (name) {
          const values = (ctx.body.fields || {})[name] || [];
          if (values.length > 1) throw new Error(name + " must be provided once");
          return values[0];
        };
        req = {};
        const fields = ctx.body.fields || {};
        for (const name of Object.keys(fields)) {
          if (name === "images") req.images = fields[name] || [];
          else req[name] = first(name);
        }
        if (req.metadata !== undefined) {
          let parsed;
          try {
            parsed = JSON.parse(req.metadata);
          } catch (e) {
            throw new Error("metadata must be a JSON object string");
          }
          if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("metadata must be a JSON object string");
          req.metadata = parsed;
        }
        if (req.seconds !== undefined) req.seconds = Number(req.seconds);
        else if (req.duration !== undefined) req.seconds = Number(req.duration);
        if (req.duration !== undefined) req.duration = Number(req.duration);
      } else throw new Error("JSON or multipart body required");
      return {
        kind: "submit",
        model: ctx.model,
        action: firstImage(req) ? "image_to_video" : "text_to_video",
        requestBody: Object.assign({}, req, { model: ctx.model }),
      };
    },
    render: function (ctx, task) {
      const data = task.data || {};
      if (data.VideoGenerationJob) {
        const job = data.VideoGenerationJob;
        const yikeStatuses = {
          Created: "queued",
          Queuing: "queued",
          Executing: "in_progress",
          Finished: "completed",
          Failed: "failed",
        };
        const yikeOutput = {
          id: task.task_id,
          object: "video",
          model: task.properties ? task.properties.origin_model_name || "" : "",
          status: yikeStatuses[job.Status] || "unknown",
          progress: Number(String(task.progress || "0").replace("%", "")),
          created_at: task.created_at,
          completed_at: task.updated_at,
        };
        if (job.Status === "Failed" && trimmed(job.ErrorMessage)) {
          yikeOutput.error = { code: "yike_task_failed", message: String(job.ErrorMessage) };
        }
        return yikeOutput;
      }
      const outputData = data.output || {};
      const statuses = {
        PENDING: "queued",
        RUNNING: "in_progress",
        SUCCEEDED: "completed",
        FAILED: "failed",
        CANCELED: "failed",
        UNKNOWN: "failed",
      };
      const output = {
        id: task.task_id,
        object: "video",
        model: task.properties ? task.properties.origin_model_name || "" : "",
        status: statuses[outputData.task_status] || "unknown",
        progress: Number(String(task.progress || "0").replace("%", "")),
        created_at: task.created_at,
        completed_at: task.updated_at,
      };
      if (data.code) output.error = { code: data.code, message: data.message || "" };
      else if (outputData.code) output.error = { code: outputData.code, message: outputData.message || "" };
      return output;
    },
  },
};
