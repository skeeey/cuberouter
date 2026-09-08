# cuberouter 分级限流与超限通知方案（per-user / per-API-key）

> 状态：方案稿（v0.1）
> 适用范围：客户需求 —— "support rate limiting configurable per user or per API key (e.g. requests per minute, tokens per minute), and how consumers are notified when limits are reached"
> 本文档同时面向客户与内部评审：前半部分为对外口径，标注 ⚙️ 的段落为内部实现口径，对外发送前请移除。

## 1. 需求概述

要求网关支持**按用户（per user）或按 API key（per API key）可配置的限流**，粒度示例为：
- 每分钟请求数（requests per minute, RPM）
- 每分钟 token 数（tokens per minute, TPM）

并要求明确**当消费者触达上限时如何被通知**。

## 2. 现有能力与差距

cuberouter 目前内置一套请求频率限制，以下是**今天即可交付的能力**与需求差距的对照：

| 需求点 | 现状 | 结论 |
|---|---|---|
| requests per minute（每分钟请求数） | ✅ **已有**：系统级请求频率限制（滑动窗口，可分别限制"总请求数"与"成功请求数"），且支持按**分组**设置不同额度；Redis 模式下全集群生效 | 基础能力具备 |
| per user 可配置 | ⚠️ **部分**：限流按**用户维度**生效（每个用户的请求独立计数），但额度是**系统级统一配置 + 分组级覆盖**，不支持"给某个具体用户单独设一个额度" | 需要增强 |
| per API key 可配置 | ❌ **无**：同一用户的所有 API key 共享其用户额度，key 本身无独立频率配置（key 上现有的 `model_limits` 是模型白名单，非频率） | 需要增强 |
| tokens per minute（每分钟 token 数） | ❌ **无**：现有限流只按请求数计数；按 token 消耗计量需要请求结算后按分钟窗口累计（流式请求按真实 usage 计入），属新能力 | 需要增强 |
| 超限通知 | ⚠️ **基本有但不够标准**：触达上限返回 **HTTP 429** + OpenAI 兼容 JSON 错误体；但缺少标准 `rate_limit_exceeded` 错误码、`Retry-After` 响应头与剩余额度头，客户端难以自动处理 | 需要增强 |

**与常见误解的区分**（答复客户时可主动澄清）：
- 额度过期 / 余额不足导致的拒绝是 **`insufficient_quota`** 语义，与频率限流（`rate_limit_exceeded`）不同，本文不混为一谈；
- 网关自身过载保护返回 **503**，也不属于限流语义。

## 3. 推荐方案：分层限流模型

在现有频率限制基础上扩展为**四层可配置额度，逐层覆盖**（与产品内额度继承、分组体系一致的思路）：

```
系统默认（现有设置，可关闭）
   └─► 分组覆盖（现有能力：组 → [RPM, TPM]）
         └─► 每用户覆盖（新增：管理员可为指定用户设置个人额度）
               └─► 每 API key 覆盖（新增：用户/管理员可为单个 key 设置额度，默认继承用户额度）
```

- **额度项**：RPM（每分钟请求数，可再拆总请求/成功请求，沿用现有语义）；TPM（每分钟输入+输出 token 数）
- **未配置即继承**：key 未设额度时继承其用户的额度，用户未设时继承所在分组/系统默认——对存量用户与 key **零配置即兼容**，不影响现网
- **生效范围**：⭐ 集群级准确限流要求启用 Redis（现有基础设施已支持）；未启用 Redis 时按单节点进程计数（多节点部署下每节点独立计数，见 §7）
- **配置入口**：管理后台系统设置 + 用户管理/密钥管理页的单条额度编辑（操作本身记入审计日志，⚙️ 复用现有操作审计链路）
- **允许"关闭限流"**：额度为 0/留空 = 继承或不限制，保持现有语义

### RPM 与 TPM 的执行时机（行为契约）

| 额度 | 检查时机 | 说明 |
|---|---|---|
| RPM | 请求入口（现有模式） | 到达即检查窗口计数，超限立即 429 |
| TPM | 入口预检 + 结算累计 | 入口按请求预估 token 预检（流式请求无法预知完整用量）；**完成后按实际 usage 计入分钟窗口**；预估不足而实际超限的请求在结算时计入，下一次请求生效——TPM 为"滚动窗口累计型"约束，不做请求中途掐断 |

> 流式（streaming）请求的 TPM 属近似约束（完成前无法得知真实 token 数），文档会向客户明确该语义，避免误解为硬性门禁。

## 4. 消费者超限通知规范

任何一层额度触达上限时，网关统一返回：

```
HTTP/1.1 429 Too Many Requests
Retry-After: <秒，建议稍大于窗口长度>
Content-Type: application/json

{ "error": {
    "message": "Rate limit exceeded for API key xxx (60 requests / minute). Retry after 61s.",
    "type": "rate_limit_exceeded",
    "code": "rate_limit_exceeded"
} }
```

配套约定（对标 OpenAI 生态惯例，兼容主流 SDK）：

1. **错误码稳定**：`type` 与 `code` 恒为 `rate_limit_exceeded`，客户端据此与配额耗尽（`insufficient_quota`）、过载（503）区分
2. **Retry-After 头**：告知下次可重试时间，客户端 SDK 可自动退避
3. **可选的额度响应头**（默认开启，按需关闭）：每次成功的 API 响应携带 `X-RateLimit-Limit`、`X-RateLimit-Remaining`、`X-RateLimit-Reset`，消费者可提前感知接近上限，主动降速/排队
4. **消息字段化**：message 说明触达的是哪一层、哪个额度（per-key/per-user、RPM/TPM、窗口长度），便于排障
5. **管理端可见**：429 事件写入日志与容量监控（现有 `rejected_429` 指标），超限可审计、可回溯
6. ⚙️ 存量问题一并修复：非 Redis（进程内）分支当前只回 429 无 body，需补齐统一错误体

## 5. 配置对象与示例

| 配置对象 | 字段 | 示例 |
|---|---|---|
| 系统默认 | RPM、TPM（按分组覆盖） | 默认 60 RPM / 无限 TPM |
| 用户 | RPM、TPM（覆盖） | 用户 `alice`：120 RPM / 200k TPM |
| API key | RPM、TPM（覆盖，可选模型维度*） | key `sk-…web`：30 RPM / 50k TPM |

\* 可选项：是否要求"按模型分别限流"（如 `gpt-4o` 与 `gpt-4o-mini` 各自计 RPM/TPM）。现有产品已按模型维度记录用量，技术上可支持，但会显著增加配置面，建议一期不做（见 §7 确认项）。

## 6. 交付范围与里程碑

| 阶段 | 内容 | 说明 |
|---|---|---|
| 一期 | per-key / per-user RPM：Token/User 额度字段 + 分层解析（默认→分组→用户→key）+ 管理后台配置与审计 + 429 通知标准化（错误码/Retry-After/额度头） | 复用现有滑动窗口/令牌桶与中间件挂载点，工作量集中在前端配置与字段迁移 |
| 二期 | TPM 计量限流：分钟窗口累计器（Redis）+ 入口预估预检 + 结算累计 | 独立于 RPM，可并行排期 |
| 三期（可选） | 按模型维度限流、用户自助查看自身额度余量 | 视客户需要 |

⚙️ 内部口径：
- Token/User 加列需按 AGENTS.md 完成 SQLite/MySQL/PostgreSQL 迁移与升级验证（新增列默认值需兼容三库）；字段进入 token 的 Redis 缓存时需同步缓存结构与失效路径（`model/token.go` 的 `cacheInitToken` 等）
- 限流中间件按 (userId/tokenId + 额度来源) 组合 Redis key，需与现有 perf 指标、容量桶（`rejected_429`）保持打通
- 分层额度解析建议收口为独立 package（如 `setting/rate_limit` 扩展），避免散落各调用点；相关行为需 middleware 层与解析层单测覆盖

## 7. 需双方确认的事项

1. 需求维度：per-user 与 per-API-key **都要**，还是仅其一？RPM 与 TPM 是否都要（TPM 为一期不可用、二期提供）？
2. 部署形态：客户网关是否为多节点？多节点下要集群级准确限流必须启用 Redis（现有能力）；是否接受单节点内存限流（各节点独立计数）作为无 Redis 时的降级？
3. 限流语义：TPM 对流式请求的"结算后累计、近似约束"语义是否可接受？
4. 通知细节：是否需要 `X-RateLimit-*` 响应头、`Retry-After` 精确到秒的窗口建议？
5. 是否要求按模型分别限流（三期），还是一期只按 key/user 总额度？
6. 现网影响：分层默认继承的设计（存量用户/key 零配置）是否符合预期？是否需要"必须显式配置才生效"的严格模式？
7. ⚙️ 内部：429 统一错误体对现有对接方的兼容性确认（现 Redis 分支 message 为中文，将切换为稳定英文 code + 字段化 message）

---

*文档维护：cuberouter · suanova · 本方案对应代码尚未实施，实施前请更新本文档状态。*
