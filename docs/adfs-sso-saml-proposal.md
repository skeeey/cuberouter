# cuberouter 对接 ADFS 单点登录（SSO）方案

> 状态：方案稿（v0.1）
> 适用范围：客户需求 —— "support ADFS identity-federation service for single sign-on"
> 本文档同时面向客户与内部评审：前半部分为对外口径，标注 ⚙️ 的段落为内部实现口径，对外发送前请移除。
> **对外答复前请先读 §7 澄清清单并回传客户确认**——方案走向取决于 IdP 实际形态，避免为错误假设做设计。

## 1. 需求概述

cuberouter 支持通过 **ADFS（Active Directory Federation Services）身份联合服务**实现单点登录（SSO）。

## 2. 协议概念确认

**ADFS 的"身份联合"主协议即 SAML 2.0。** ADFS 是微软的联合身份服务器，企业将其用于 SSO（尤其对接第三方系统）时，绝大多数部署走 SAML 2.0 断言。因此客户所述 "ADFS identity-federation service for SSO" 按业界惯例应理解为：**以 ADFS 为 SAML 身份提供方（IdP），cuberouter 作为服务提供方（SP）接入**。

补充两个边界，避免理解偏差：

- ADFS 同时支持旧式 **WS-Federation / WS-Trust**（服务微软自家旧产品线），属历史协议，一般不作为新对接首选；
- **ADFS 2016+ 提供功能受限的 OIDC/OAuth2 支持**（无 UserInfo 端点、无动态客户端注册等）——若客户实际期望 OIDC 对接，须明确 ADFS 版本并按下述 §4.2 评估；
- ⚠️ **常见场景误认**：客户写 "ADFS" 时常实际指 **Microsoft Entra ID（原 Azure AD）** 或 O365 账号体系。Entra ID 是标准 OIDC IdP，与 ADFS 形态、协议、对接路径完全不同（见 §5）（需澄清）。

## 3. cuberouter 现有认证能力与差距

**今天已有的认证/登录方式**：本地账号 + 密码（JWT 会话）、WebAuthn/Passkey、OAuth2 系第三方登录（GitHub / Discord / LinuxDO / **通用 OIDC** / 可自定义的通用 OAuth Provider）。

| 客户场景 | 产品现状 | 结论 |
|---|---|---|
| ADFS 以 **SAML 2.0 IdP** 形态对接 | **无 SAML 支持**（代码库中无任何 SAML/WS-Fed 引用；`oauth/` 全部为 OAuth2 系 Provider） | ❌ 需新增能力（§4.1） |
| ADFS 2016+ 开启 **OIDC 端点** | 通用 OIDC Provider 存在（端点可配置） | ⚠️ 有条件：ADFS 无 UserInfo 端点，现有 OIDC 实现强依赖 userinfo 返回 `sub`+`email`，需小幅适配（§4.2） |
| 实际为 **Microsoft Entra ID** | 通用 OIDC 完整支持（Entra 为标准 OIDC，含 UserInfo） | ✅ 现成可用（§5） |

## 4. 推荐方案

### 4.1 主推：SAML 2.0 SP 接入（ADFS 标准形态）

cuberouter 作为 SAML 服务提供方（SP），与客户 ADFS（IdP）建立信任：

```
┌─────────────┐  1. 用户访问 cuberouter，点击 "SSO (ADFS)" 登录
│  浏览器      │ ─────────────────────────────────────────────┐
└─────┬───────┘                                              ▼
      │                                           ┌──────────────────────┐
      │     2. 重定向（SAML AuthnRequest，POST）     │  客户 ADFS (IdP)      │
      │ ─────────────────────────────────────────► │  域账号认证 + 断言签发  │
      │                                            └──────────────────────┘
      │     3. 断言回传（SAMLResponse，POST → /saml/acs）
      ▼ ─────────────────────────────────────────
┌─────────────────────────────────────────────────────────────┐
│ cuberouter SAML SP：验证断言签名 → 映射 Claims → 建号/绑定/登录 │
└─────────────────────────────────────────────────────────────┘
```

**接入内容**：

1. **SAML SP 能力**：引入 Go SAML SP 库（如 `crewjam/saml`），支持 SP 元数据导出、HTTP-POST 绑定断言消费端点（`/saml/acs`）；支持服务提供商证书签发与**客户 ADFS 证书信任**（签名验证、断言防重放：`NotOnOrAfter` + 会话内 Assertion ID 去重）
2. **元数据交换**：我方提供 SP 元数据 URL 给客户配置 Relying Party Trust；客户提供 IdP 元数据（含签名证书）导入系统——证书轮换走元数据更新，不中断业务
3. **Claims 映射**（对齐 ADFS 常见属性）：

| ADFS 断言 | cuberouter 用途 |
|---|---|
| NameID（建议 persistent 格式） | 用户唯一关联键（类似现有 `oidc_id` 的绑定列） |
| `email` / `upn` | 建号与登录名 |
| `displayname` / `givenname` | 显示名（可选） |
| 组声明（SID/名称） | 可选：映射本地分组/角色（需与客户对组清单） |

4. **建号与绑定策略**（确认项，见 §7）：首次登录自动建号（JIT）或仅允许预置账号；绑定唯一性与现有账号绑定/审计链路一致（登录事件入审计日志）
5. **会话与退出**：登录成功后沿用现有会话机制；**SLO（单点退出）一期不做**（由 cuberouter 本地会话退出 + ADFS 自身空闲超时兜底），作为确认项
6. **配置与运维 UI**：管理后台配置 SP 证书、IdP 元数据、启用开关；配置变更入审计日志

### 4.2 备选路径：ADFS 的 OIDC 端点（若客户确认此形态）

- 现有通用 OIDC Provider 基础上做小幅适配：ADFS 不提供 UserInfo 端点，需支持**解码 `id_token`（JWT）提取 claims**（当前实现 `oauth/oidc.go` 的 GetUserInfo 强制请求 userinfo 且要求 `sub`+`email` 非空，需为无 UserInfo 的 IdP 增加 id_token 解析分支）
- **约束**：ADFS OIDC 无标准 UserInfo；claims 依赖 ADFS 侧规则；仅 ADFS 2016+ 可用

### 5. 若实际是 Microsoft Entra ID

**现有功能直接支持，无需开发**：管理后台启用通用 OIDC，填写 Entra 租户的授权/Token/UserInfo 端点与 Client ID/Secret 即可；账号绑定、自动建号、审计均沿用现有 OIDC 链路。此路径应作为澄清后的第一优先排查项。

## 6. 交付范围与里程碑

| 阶段 | 内容 | 说明 |
|---|---|---|
| 一 | SAML SP：元数据交换、断言消费、签名验证与防重放、Claims 映射建号/绑定、登录 UI 与审计 | 不含 SLO、不含组→角色映射（视确认项） |
| 二 | SLO、组声明→分组/角色映射、证书轮换自助 UI | 视客户要求 |

⚙️ 内部口径：
- SAML 库选型需评审（维护活跃度、许可证、对 Go 版本要求），并过 AGENTS.md 依赖管理流程；认证链路安全敏感，建议独立安全评审（签名强制验证、禁止接受未签名断言、XML 外部实体防护、断言消费端点防重放与防无限建号）
- 用户绑定列/绑定表沿用现有模式（`oidc_id` 列或 `user_oauth_bindings`/`external_identity_claims` 通用表），涉及 SQLite/MySQL/PostgreSQL 三库迁移验证
- Claims 映射与建号逻辑收口为独立 service/oauth 包内适配器（SAML Provider 实现现有 `oauth.Provider` 接口），前端登录按钮与既有 OAuth 登录并列
- i18n：新增认证相关文案需同步后端 en/zh 与前端语言文件（沿用现有同步工具）

## 7. 需向客户确认的澄清清单

> 对外答复与方案推进前，请先请客户逐条回传（不确定的可标"待我方确认"）。

1. **IdP 实际形态**：是自建 ADFS，还是 Microsoft Entra ID（Azure AD）/ O365？——若为 Entra ID，现有 OIDC 能力直接支持，无需 SAML 开发
2. **ADFS 版本与部署形态**：版本（2012 R2 / 2016 / 2019 / 2022）？单机还是场（farm）？是否为 ADFS Proxy/WAP 暴露？
3. **协议期望**：SAML 2.0（标准）还是期望走 ADFS 的 OIDC 端点？（若仅需 SSO 且 ADFS 较新，两条路径均可，需客户选型）
4. **建号策略**：首次 SSO 登录是否允许自动创建本地账号（JIT）？还是仅限管理员预置账号？
5. **属性与组映射**：ADFS 侧可提供的断言属性（upn / email / displayName / 组）；是否要求组声明映射到 cuberouter 分组或角色？如需要，提供组清单与目标角色对应关系
6. **退出行为**：是否要求单点退出（SLO）？还是一期接受 cuberouter 本地退出 + ADFS 空闲超时？
7. **证书与运维**：ADFS 签名证书轮换周期与通知责任人（我方信任链需跟随更新）；SP 证书由我方生成还是客户签发？
8. **登录入口**：是否需要独立登录页按钮（"使用企业账号登录"）与本地账号登录并列，还是仅允许 SSO？
9. **合规与审计**：登录/建号审计是否需含 ADFS 返回的 NameID/upn 原文（涉及个人数据留存策略）？
10. **对接窗口与联调环境**：客户是否可提供联调用 ADFS（或测试租户）与我们交换元数据？期望的上线时间？

---

*文档维护：cuberouter · suanova · 本方案对应代码尚未实施，实施前请更新本文档状态。*
