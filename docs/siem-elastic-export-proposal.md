# cuberouter 日志与审计数据对接 Elastic SIEM 方案

> 状态：方案稿（v0.1）
> 适用范围：客户合规/安全运营需求 —— "export logs and audit data to Elastic SIEM"
> 本文档同时面向客户与内部评审：前半部分为对外口径，标注 ⚙️ 的段落为内部实现口径，对外发送前请移除。

## 1. 需求概述

要求将 cuberouter 网关产生的**日志数据**与**审计数据**持续导出至其 Elastic SIEM（Elastic Security），用于安全监控、合规留痕与威胁检测。

对应到产品侧，数据范围划分为：

| 客户视角 | 产品数据 |
|---|---|
| 使用/请求日志（usage logs） | 每次模型调用的消费、错误、退款记录（含用户、模型、Token 用量、渠道、分组、请求 ID、耗时） |
| 异步任务日志（task logs） | 图像/视频等异步任务的生命周期与计费记录 |
| 审计数据（audit data） | 管理操作审计（用户/渠道/模型/配置变更，含操作者与动作参数）、登录日志、充值日志、系统日志 |

## 2. 产品数据现状（可导出的数据资产）

cuberouter 的日志与审计数据统一存储于 `logs` 表（可独立配置存储位置），日志类型通过 `type` 字段区分：

- 充值 `topup`、消费 `consume`、管理操作 `manage`（操作审计）、系统 `system`、错误 `error`、退款 `refund`、登录 `login`
- 审计日志已结构化：`Other.op.action`（稳定动作标识）+ `Other.op.params`（结构化参数），另含 `admin_info`（操作者身份）、`audit_info`（路由/方法等）——适合直接映射为 SIEM 的事件字段，不依赖自然语言
- 每条记录含 `request_id`（请求关联 ID）、`username`、`ip`（按策略）、`model_name`、token 用量、`channel`、分组等维度
- 任务日志为独立数据集，含任务 ID、状态、时长、失败原因等

结论：**产品内数据完整、结构化，无需改造记录链路即可支撑 SIEM 导出**，核心工作是提供安全、可靠的对外导出通道与 Elastic 侧的标准接入配置。

## 3. 推荐方案总览

采用**拉取式 SIEM 导出接口 + 客户侧 Elastic 采集**的组合（业内对接 Elastic SIEM 的标准做法）：

```
┌─────────────────────────┐        HTTPS + Bearer Token          ┌──────────────────────────┐
│  cuberouter 网关          │ ◄────────── 定时轮询 ──────────────── │  Elastic Agent            │
│  SIEM Export API          │          (httpjson input)           │  / Logstash http input    │
│  GET /api/siem/events     │                                     └───────────┬──────────────┘
│  输出 ECS 风格 JSON Lines │                                                 │ 内部网络
└─────────────────────────┘                                                 ▼
                                                              ┌──────────────────────────┐
                                                              │  Elasticsearch + Security │
                                                              │  （客户已有/新建索引）      │
                                                              └──────────────────────────┘
```

**产品侧（cuberouter）**：提供只读、Token 鉴权的 SIEM 导出端点，按"游标续传 + 时间窗口"返回增量事件，每条事件为一条 Elastic Common Schema（ECS）风格 JSON。产品不接触客户的 Elastic 集群，不保存任何 Elastic 凭据。

**客户侧（Elastic）**：使用 Elastic Agent 的 HTTP JSON input（或 Logstash HTTP input）定时拉取该端点，事件经索引模板落入 Elasticsearch，由 Elastic Security 统一消费。拉取、重试、断点续传由 Agent 负责，无需客户编写脚本。

> 与产品内既有「Prometheus 指标拉取接口」（`/metrics`，Bearer Token 鉴权，关闭时返回 404）为同一范式，运维习惯一致。

### 为什么不是「网关主动推送」

| | 拉取式导出（推荐） | 网关主动推送（备选） |
|---|---|---|
| Elastic 侧配置 | 标准 httpjson/Logstash input，复用现有 Fleet | 需向产品配置 Elasticsearch/Logstash 地址与 API Key |
| 凭据管理 | 产品只发放自己的导出 Token | 产品需保存客户侧凭据，存在保管/轮换义务 |
| 可靠性 | 重试、背压、续传由 Agent 承担 | 推送失败/积压/顺序需产品自行处理 |
| 适用网络 | 要求客户网络可回连网关 | Elastic 位于 NAT/隔离网段时可用 |

若个别客户网络方向不允许回连，可在二期提供推送模式（Logstash HTTP input / Elasticsearch bulk API），导出事件格式保持一致，客户无感切换。

## 4. 导出接口设计（对外规格）

### 4.1 端点

```
GET /api/siem/events
Authorization: Bearer <SIEM_EXPORT_TOKEN>
```

| 参数 | 说明 |
|---|---|
| `since` | 上次同步位置（游标），首拉可省略（默认取配置的起始时间） |
| `types` | 可选，事件类型子集（`usage`/`audit`/`task`/`all`，默认 `all`） |
| `limit` | 单次返回条数上限（默认 1000，上限 10000） |

响应：`application/x-ndjson`，每行一个 JSON 事件；末尾行为元信息行 `{"cursor":"...","has_more":true|false}`，调用方以 `cursor` 作为下次 `since`。

游标为不透明字符串（内部为 `(时间, request_id, type)` 复合键），保证**不重不漏、可断点续传**，且不依赖数据库自增 ID（兼容 ClickHouse 等无单调 ID 的日志存储）。

### 4.2 事件格式（ECS 风格，示意）

审计事件样例：

```json
{
  "@timestamp": "2026-09-08T03:14:02.000Z",
  "event": {
    "kind": "event", "category": ["iam"], "type": ["change", "user"],
    "action": "user_update", "outcome": "success",
    "id": "req_8f3a…", "dataset": "gateway.audit", "module": "cuberouter"
  },
  "user": { "name": "admin01", "id": "3" },
  "source": { "ip": "203.0.113.7" },
  "observer": { "name": "node-a" },
  "message": "user updated by admin01",
  "cuberouter": { "action_params": { "user_id": 42, "fields": ["quota"] } }
}
```

使用日志样例：

```json
{
  "@timestamp": "2026-09-08T03:14:01.000Z",
  "event": {
    "kind": "metric", "category": ["billing"], "type": ["info"],
    "action": "consume", "outcome": "success",
    "id": "req_8f3a…", "dataset": "gateway.usage", "module": "cuberouter"
  },
  "user": { "name": "alice" },
  "source": { "ip": "198.51.100.9" },
  "http": { "request": { "method": "POST" } },
  "cuberouter": {
    "model": "gpt-4o", "group": "default", "channel_id": 7, "token_name": "web",
    "prompt_tokens": 120, "completion_tokens": 340, "quota": 46000,
    "use_time_ms": 1820, "stream": true, "upstream_request_id": "chatcmpl-…"
  }
}
```

> 字段名以最终交付的映射表为准；`event.action` 直接复用审计日志的稳定动作标识（如 `user_update`/`channel_delete`/`login`），客户可在 Elastic Security 中直接基于 `event.action` 配置检测规则，不受界面语言影响。

## 5. 客户侧接入指引（交付物摘要）

1. **启用导出**：管理后台 → 系统设置 → SIEM 导出，开启并生成只读 Token（可轮换）；按需限定事件类型与起始时间。
2. **网络**：确保 Elastic Agent/Logstash 所在节点可访问网关导出端点（HTTPS，建议 TLS 1.2+）。
3. **配置 Elastic Agent**（示意，Fleet 托管则录入集成策略）：

```yaml
inputs:
  - type: httpjson
    enabled: true
    data_stream.namespace: gateway
    interval: 10s
    request.url: https://gateway.example.com/api/siem/events
    request.method: GET
    request.headers:
      Authorization: Bearer ${SIEM_EXPORT_TOKEN}
    request.transforms:
      - set:
          target: url.params.since
          value: '[[.cursor.next]]'
          default: ''
    response.split:
      target: body
      split:
        keep_parent: true
    response.pagination:
      - set:
          target: url.params.since
          value: '[[.last_response.body.cursor]]'
          when: '[[.last_response.body.has_more]] == true'
```

4. **索引与保留**：交付 ECS dataset 索引模板（`logs-gateway.audit-*`、`logs-gateway.usage-*`、`logs-gateway.task-*`），数据保留期由客户按合规要求配置 ILM。
5. **验证**：在 Elastic Security 中检索 `event.dataset : gateway.*`，确认事件持续入库；建议首拉历史窗口完成一次对账。

## 6. 安全与隐私

- **鉴权**：导出端点为独立 Token（Bearer，恒定时间比较），关闭时返回 404 不暴露存在性——沿用产品既有 `/metrics` 范式
- **数据边界**：导出内容为**完整管理视图**（含操作者信息与审计细节），仅限获得 Token 的合规/SOC 侧使用；普通用户日志接口的脱敏规则不适用于此通道
- **IP 记录策略**：当前按用户设置决定是否记录 IP。合规场景如需保证 `source.ip` 完整，建议在导出开启时对相应日志强制采集 IP（⚙️ 该点为产品决策项，见 §8）
- **敏感内容**：导出不含对话/提示词原文（`content` 仅为结构化元数据），如需调整以最终映射表为准
- **传输**：仅支持 HTTPS 访问导出端点

## 7. 交付范围与里程碑

| 阶段 | 内容 | 说明 |
|---|---|---|
| 一期 | SIEM 导出 API（鉴权/游标/类型过滤/限流）+ ECS 映射 + 管理后台设置 + 接入文档与 Agent 配置示例 + 索引模板 | 覆盖全部日志与审计类型；拉取式 |
| 二期（可选） | 推送模式（Logstash HTTP / ES bulk API），按客户网络要求提供 | 事件格式与一期一致 |
| 运维配套 | 导出 Token 轮换、启用状态审计（导出开启/关闭本身记录审计日志）、指标暴露（拉取延迟/失败数） | — |

⚙️ 内部口径：需通过 AGENTS.md 规定的数据库兼容验证（SQLite/MySQL/PostgreSQL/ClickHouse 日志存储下分别验证游标查询与迁移），并补充 controller/model 层测试。

## 8. 需双方确认的事项

1. 数据范围：`usage + audit` 是否足够，还是需要全部类型（含任务日志/充值/系统日志）？
2. 事件量级：峰值每秒日志行数约为多少？决定单批上限与拉取间隔（高量场景可考虑仅导出 audit 或错误日志子集）
3. 客户 Elastic 形态：Elastic Cloud 还是自建？是否已有 Fleet/Elastic Agent 基础设施？由谁维护 Agent 配置？
4. 网络方向与传输安全：是否允许 Elastic 侧访问网关？是否要求 mTLS/私有 CA？
5. 数据保留：SIEM 侧索引保留期要求（对产品无影响，仅影响客户侧 ILM）
6. ⚙️ IP 强制采集是否为可接受的策略变更（影响 `RecordIpLog` 现有按用户开关的设计）
7. ⚙️ 是否需要支持从某个历史时间点全量初始化导出（确定首拉窗口与是否限量）

---

*文档维护：cuberouter · suanova · 本方案对应代码尚未实施，实施前请更新本文档状态。*
