# GoLineage — 企业级资产与全路径血缘分析图谱
## 需求规格说明书（Requirements SSOT）

| 项 | 值 |
|---|---|
| 项目代号 | GoLineage |
| 文档版本 | v1.0（需求冻结） |
| 冻结时间 | 2026-08-21 20:05 (GMT+8) |
| 负责阶段 | PM Agent — Alkaid-SOP v13.0 |
| 上游 SSOT | `docs/.meta/original_prompt.md` |
| 下游文档 | `docs/Roadmap.md`（待 Phase 1 生成） |

> 本文件定义 **WHAT**（做什么）。`docs/Roadmap.md` 定义 **WHEN**（何时做）。
> 任何代码实现均不得超出本文件范围（Redline 4：禁止范围漂移）。

---

## 1. 废除评估结论（Abandonment Assessment）

| # | 判据 | 检查结果 | 结论 |
|---|---|---|---|
| 1 | 不完整 / 模糊 | 主体明确（资产血缘图谱），无缺失附件依赖，前后端职责清晰 | ✅ 通过 |
| 2 | Windows 专属 | Go + Vue3 + Docker，全链路 POSIX / 跨平台 | ✅ 通过 |
| 3 | 规模评估（分级） | 后端 5500–8000 行（用户指定）+ 前端约 3500–4500 行 ≈ **10000–12500 LoC**，落入 10k–40k 区间 | ⚠️ **ACCEPT + 强制分期** |
| 4 | 外部依赖（智能检查） | 无任何外部第三方 API（无支付/地图/短信/AI 生成）。数据全部自持自产 | ✅ 通过，**无需 Mock Provider** |
| 5 | 专业 / 小众 | Go（BSD）、Vue 3（MIT）、AntV G6（MIT）、PostgreSQL（PostgreSQL License）均为开源免费 | ✅ 通过 |

### 🟢 最终裁决：**ACCEPT（接受立项）**

**判据 3 的强制约束**：本项目规模超过 10k LoC，依据 SOP v13 §4，
**必须先产出带 MVP / V1 / V2 明确边界的 `docs/Roadmap.md`，方可写第一行业务代码。**

---

## 2. 兼容性与矛盾检测（Contradiction Detection）

原始 Prompt 中存在 **5 处技术歧义或逻辑冲突**，PM 已逐一裁决并冻结。开发阶段不得推翻。

### ⚠️ 矛盾 C-1（严重）：「纯内存图」 vs 「历史拓扑时间轴回溯」

- **冲突描述**：Prompt 要求「手写内存图结构」，同时要求「记录完整变更流水，支持按时间轴回溯历史拓扑」。纯内存结构在进程重启后全量丢失，物理上无法提供历史回溯能力。
- **裁决（冻结）**：采用 **CQRS + Event Sourcing** 架构。
  - **写路径（Command）**：所有拓扑变更先以不可变事件（Event）持久化到 PostgreSQL 事件日志表，再应用到内存图。
  - **读路径（Query）**：内存图作为高性能只读投影（Read Model），承载 BFS / DFS / Dijkstra。
  - **回溯**：给定时间戳 T，从事件日志重放（Replay）至 T，在独立内存实例中重建历史快照。
  - **启动恢复**：进程启动时全量重放事件日志重建内存图，保证重启不丢数据。
- **理由**：这是唯一能同时满足「手写内存图性能」与「历史可回溯」两项硬要求的架构，而非二选一妥协。

### ⚠️ 矛盾 C-2：「邻接表 / 邻接矩阵」二选一未定

- **冲突描述**：两种结构的空间复杂度差异为 O(V+E) vs O(V²)。企业资产图为典型稀疏图（平均度数 < 20），邻接矩阵在 10 万节点下需 10¹⁰ 单元，不可行。
- **裁决（冻结）**：**主结构使用邻接表**（`map[NodeID]*adjacencyList`，出边 / 入边双向索引）。
  邻接矩阵**仅**在「子图密度分析」场景下按需对 ≤ 500 节点的子图临时构建，作为算法教学与稠密子图加速的补充实现，不作为主存储。

### ⚠️ 矛盾 C-3：「AntV G6 / Cytoscape.js」二选一未定

- **裁决（冻结）**：**AntV G6 v5**。
- **理由**：① Vue 3 集成生态成熟；② 内置 force / dagre / radial 多种布局，血缘图（DAG 形态）需要 dagre 分层布局，G6 原生支持；③ 中文文档完备，降低维护成本；④ MIT 协议无商用风险。
- Cytoscape.js 明确**不引入**，避免双图引擎导致包体积翻倍。

### ⚠️ 矛盾 C-4：Dijkstra 边权语义未定义

- **冲突描述**：Dijkstra 依赖非负边权，但「服务器→数据库」这类资产关系本身无天然权重。
- **裁决（冻结）**：`Edge.Weight float64` 显式建模，默认值 `1.0`（退化为 BFS 等价的跳数最短路）。
  权重业务语义定义为 **「依赖代价」**，取值范围 `(0, +∞)`，可由用户在边属性中自定义（如调用平均延迟 ms、故障传导概率倒数）。
  **约束**：写入时校验 `Weight > 0`，负权直接拒绝（HTTP 400），不引入 Bellman-Ford。

### ⚠️ 矛盾 C-5：「图数据库适配」是否引入 Neo4j

- **裁决（冻结）**：**不引入任何外部图数据库**。
  但在代码层定义 `GraphRepository` 接口（Port），内存实现为 `MemoryGraphAdapter`。
  接口的存在即为「图数据库适配」的落地形态 —— 未来可新增 Neo4j Adapter 而无需改动 Service 层。
- **理由**：引入 Neo4j 将使「手写高性能内存图结构」这一核心考核点失去意义，且违反 Redline 4（范围漂移）。

### ✅ 交付标准兼容性检查

| 检查项 | 结论 |
|---|---|
| 微信小程序例外 | 不适用（Web 项目） |
| Docker 交付标准（Redline 1） | ✅ 满足。Go 后端 + Nginx 静态前端 + PostgreSQL 三容器，`docker compose up` 一键启动 |
| 跨平台（ARM64 / AMD64） | ✅ 满足。`golang:1.25-alpine`、`node:21-alpine`、`nginx:alpine`、`postgres:17-alpine` 均提供双架构镜像 |
| localhost 可访问 | ✅ 前端 Web UI 通过浏览器直接访问 |
| 美学卓越（Redline 2） | ✅ 强制要求，见 §5.6 |

---

## 3. 技术栈冻结（Frozen Stack）

| 层 | 技术选型 | 版本 | 冻结理由 |
|---|---|---|---|
| 后端语言 | Go | 1.25 | 用户指定 |
| HTTP 框架 | `net/http` + `chi` 路由 | chi v5 | 轻量，贴近标准库，不引入重型框架掩盖手写图结构这一核心 |
| 持久化 | PostgreSQL | 17-alpine | 事件日志需事务与 JSONB 属性存储 |
| DB 驱动 | `pgx/v5` | v5 | 原生 JSONB 支持，性能优于 lib/pq |
| 迁移 | 纯 SQL 脚本 + 启动时幂等执行 | — | 避免引入 migrate CLI 增加容器复杂度 |
| 前端框架 | Vue 3（Composition API + `<script setup>`） | 3.5 | 用户指定 |
| 构建 | Vite | 6 | Vue 3 官方推荐 |
| 语言 | TypeScript | 5.x | 图数据结构复杂，强类型必需 |
| 图可视化 | AntV G6 | v5 | 见 C-3 裁决 |
| UI 组件 | Element Plus | 2.x | 抽屉 / 表单 / 时间轴组件齐备，中文生态 |
| 样式 | TailwindCSS | 3.x | Redline 2 要求现代设计系统 |
| 状态管理 | Pinia | 2.x | Vue 3 官方 |
| 容器 | Docker Compose | v2 | Redline 1 |
| 时区 | `Asia/Shanghai` (GMT+8) | — | 工作区时区规范，容器 `TZ` 环境变量 + Go 侧统一时间工具 |

---

## 4. 领域模型（Domain Model）

### 4.1 资产节点（Node）

| 字段 | 类型 | 说明 | 约束 |
|---|---|---|---|
| `id` | string (UUID) | 节点唯一标识 | 主键，服务端生成 |
| `name` | string | 显示名称 | 必填，1–128 字符 |
| `type` | enum | 资产类型 | 见下方枚举 |
| `properties` | map[string]any | **动态属性**（IP / 责任人 / 风险等级…） | JSONB，键 ≤ 64 字符，单节点 ≤ 50 个键 |
| `created_at` | timestamp | 创建时间（GMT+8） | 服务端生成 |
| `updated_at` | timestamp | 最后更新时间（GMT+8） | 服务端维护 |
| `deleted_at` | timestamp \| null | 软删除标记 | 事件溯源要求：永不物理删除 |

**资产类型枚举**（`type`）：`server` / `database` / `application` / `api` / `person` / `account` / `middleware` / `storage`
> 前 4 类覆盖 Prompt 中的「服务器-数据库-应用-接口」IT 资产场景；
> `person` / `account` 覆盖「金融反欺诈人际关系网络」场景。

### 4.2 血缘关系边（Edge）

| 字段 | 类型 | 说明 | 约束 |
|---|---|---|---|
| `id` | string (UUID) | 边唯一标识 | 主键 |
| `source_id` | string | 起点节点 ID | 外键，必须存在且未删除 |
| `target_id` | string | 终点节点 ID | 外键，必须存在且未删除 |
| `relation` | enum | 关系类型 | 见下方枚举 |
| `weight` | float64 | 依赖代价 | **必须 > 0**，默认 1.0（见 C-4） |
| `directed` | bool | 是否有向 | 默认 true |
| `properties` | map[string]any | 边动态属性 | JSONB |
| `created_at` / `updated_at` / `deleted_at` | timestamp | 同上 | — |

**关系类型枚举**（`relation`）：`deploys_on` / `calls` / `reads_from` / `writes_to` / `depends_on` / `owns` / `transfers_to` / `associates_with`

**完整性约束**：
- 禁止自环（`source_id == target_id`）→ HTTP 400
- 禁止重复边（同 source + target + relation 且未删除）→ HTTP 409
- 删除节点时，其所有关联边级联软删除（生成独立的边删除事件）

### 4.3 变更事件（LineageEvent）— 血缘溯源核心

| 字段 | 类型 | 说明 |
|---|---|---|
| `seq` | int64 | 全局单调递增序列号（BIGSERIAL），重放的唯一顺序依据 |
| `event_type` | enum | 事件类型 |
| `entity_type` | enum | `node` / `edge` |
| `entity_id` | string | 目标实体 ID |
| `payload` | JSONB | 事件载荷（新值） |
| `before` | JSONB \| null | 变更前快照（用于 diff 展示与理论回滚） |
| `actor` | string | 操作者标识 |
| `reason` | string | 变更原因（如「A 应用下线，不再调用 B 数据库」） |
| `occurred_at` | timestamp | 事件发生时间（GMT+8，时间轴回溯的检索键） |

**事件类型枚举**：`node_created` / `node_updated` / `node_deleted` / `edge_created` / `edge_updated` / `edge_deleted`

**不可变性约束（硬性）**：事件表**只允许 INSERT**，禁止 UPDATE / DELETE。此约束在 DB 层通过权限或触发器强制。

---

## 5. 功能需求（Functional Requirements）

### 5.1 FR-A：手写内存图引擎（后端核心，最高优先级）

| ID | 需求 | 验收方式 |
|---|---|---|
| FR-A1 | 基于邻接表的有向带权图结构，维护出边表与入边表双向索引 | 单元测试 |
| FR-A2 | 节点 / 边的增删改，全部操作并发安全（`sync.RWMutex`，读多写少优化） | 竞态测试 `go test -race` |
| FR-A3 | **BFS** 遍历：支持指定起点、最大深度、方向（出/入/双向）、关系类型过滤 | 单元测试 + API |
| FR-A4 | **DFS** 遍历：同上过滤能力，且必须检测并安全处理环路 | 含环图单元测试 |
| FR-A5 | **Dijkstra** 最短路径：基于手写二叉堆优先队列，返回完整路径节点序列 + 总代价 | 单元测试（含已知答案图） |
| FR-A6 | **全路径枚举**：枚举两点间所有简单路径（带路径数上限与深度上限保护） | 单元测试 |
| FR-A7 | **邻居子图提取**：给定节点返回 N 跳邻居子图（前端点击高亮的数据源） | API |
| FR-A8 | **上下游血缘分析**：有向可达性计算，分别返回全部上游（影响源）与下游（影响面） | API |
| FR-A9 | 二级索引：按 `type`、按 `name` 前缀、按属性键值的快速检索 | 单元测试 |
| FR-A10 | 邻接矩阵子图分析（≤ 500 节点），用于稠密度 / 连通分量计算 | 单元测试 |

### 5.2 FR-B：血缘变更溯源与时间轴回溯

| ID | 需求 |
|---|---|
| FR-B1 | 所有写操作在同一事务内先写事件日志，再应用内存图；写日志失败则整体回滚，内存图不变更 |
| FR-B2 | 事件流水查询 API：支持按实体 ID、事件类型、时间范围、操作者分页过滤 |
| FR-B3 | **历史拓扑重建**：`GET /api/v1/timeline/snapshot?at=<RFC3339>` 重放事件返回该时刻的完整拓扑 |
| FR-B4 | **拓扑 Diff**：给定两个时间点，返回新增 / 删除 / 修改的节点与边集合 |
| FR-B5 | 进程启动时全量重放事件日志重建内存图，保证重启后数据一致 |
| FR-B6 | 重放性能保护：快照检查点机制（每 N 个事件落一次全量快照，重放从最近检查点开始） |

### 5.3 FR-C：REST API 层

| ID | 需求 |
|---|---|
| FR-C1 | 节点 CRUD：`POST/GET/PUT/DELETE /api/v1/nodes` |
| FR-C2 | 边 CRUD：`POST/GET/PUT/DELETE /api/v1/edges` |
| FR-C3 | 图查询：`/api/v1/graph/neighbors`、`/shortest-path`、`/all-paths`、`/traverse`、`/lineage` |
| FR-C4 | 全量拓扑：`GET /api/v1/graph` （带节点数上限与类型过滤保护） |
| FR-C5 | 时间轴：`/api/v1/timeline/events`、`/timeline/snapshot`、`/timeline/diff` |
| FR-C6 | 健康检查：`GET /healthz`（含 DB 连通性 + 内存图节点数） |
| FR-C7 | 统一响应包络：`{ code, message, data, trace_id }`；统一错误码表 |
| FR-C8 | **API 文档**：独立 `docs/API.md`，每个端点含请求/响应示例、参数类型、错误码表（全局记忆规则 3 强制） |

### 5.4 FR-D：前端交互拓扑图

| ID | 需求 |
|---|---|
| FR-D1 | G6 画布渲染拓扑图，节点按 `type` 差异化图标与配色，边按 `relation` 差异化样式 |
| FR-D2 | 节点**拖拽**改变位置；画布**平移**；滚轮**缩放**（含缩放比例指示与「适应画布」按钮） |
| FR-D3 | **点击节点高亮相邻节点**：一跳邻居高亮，非相关元素降透明度淡出 |
| FR-D4 | **最短路径高亮**：选择起点与终点，调用后端 Dijkstra，路径链路以强调色 + 流动动画高亮，并展示总代价与跳数 |
| FR-D5 | 布局切换：力导向（force）/ 分层（dagre，血缘 DAG 默认）/ 环形（radial） |
| FR-D6 | 节点搜索与定位（按名称/类型），命中后画布聚焦 |

### 5.5 FR-E：动态节点编辑器（右侧抽屉）

| ID | 需求 |
|---|---|
| FR-E1 | 点击节点从右侧滑出抽屉，展示基础信息与全部动态属性 |
| FR-E2 | **动态增删属性**：自由添加键值对（如 `ip` / `owner` / `risk_level`），无需后端 schema 变更 |
| FR-E3 | 预设属性模板：按资产类型提供推荐属性（server → IP/机房/规格；person → 身份证/手机号/风险评分），一键填充 |
| FR-E4 | `risk_level` 属性具备语义渲染：`high`/`medium`/`low` 映射为红/黄/绿标签，并反映到画布节点描边 |
| FR-E5 | 抽屉内展示该节点的**变更历史时间线**（该实体的事件流水） |
| FR-E6 | 表单校验：键名格式、必填、重复键检测；保存失败给出明确错误提示 |

### 5.6 FR-F：时间轴回溯 UI

| ID | 需求 |
|---|---|
| FR-F1 | 底部时间轴滑块，拖动即请求该时刻历史快照并重绘画布 |
| FR-F2 | 「实时 / 历史」模式切换，历史模式下画布置为只读并显示醒目状态条 |
| FR-F3 | 变更流水面板：按时间倒序列出事件，含操作者、原因、before/after diff |
| FR-F4 | 两时间点对比模式：新增元素绿色、删除元素红色虚线、修改元素黄色标记 |

### 5.7 FR-G：美学与体验（Redline 2 强制）

- 深色科技风为主色调（图谱类产品行业惯例），Tailwind 设计令牌统一管理调色板与间距
- 全部交互具备反馈态：loading 骨架屏、空状态插画、错误态、成功 toast
- 响应式：≥ 1280px 完整布局；768–1280px 抽屉转全屏；< 768px 提示建议桌面端访问
- 禁止「工程师 UI」：无裸 HTML 控件、无错位、无未对齐边距

---

## 6. 非功能需求与可度量验收基线（NFR）

> v13 要求：有行业基准的维度必须写成**可度量**的验收标准，而非形容词。

### 6.1 性能基线（在 10 万节点 / 50 万边的种子图上，单容器 2C4G 环境测量）

| 指标 | 基线要求 | 测量方式 |
|---|---|---|
| 内存图构建（冷启动重放） | ≤ 15 s | 启动日志计时 |
| 单节点 1 跳邻居查询 | P95 ≤ 20 ms | Go benchmark |
| BFS 3 跳子图（≤ 5000 节点） | P95 ≤ 80 ms | Go benchmark |
| Dijkstra 最短路径（任意两点） | P95 ≤ 200 ms | Go benchmark |
| 单节点写入（含事件持久化） | P95 ≤ 50 ms | API 压测 |
| 内存占用（10 万节点 / 50 万边） | ≤ 1.5 GB RSS | `docker stats` |
| 前端首屏渲染（2000 节点画布） | ≤ 3 s | Lighthouse / 手动计时 |

> **降级说明**：若 QA 环境资源不足以承载 10 万节点，允许以 1 万节点 / 5 万边规模测量，
> 并在 `docs/QA_Record.md` 中显式记录降级规模与实测数据，**不得省略测量**。

### 6.2 可靠性

- 进程重启后拓扑与重启前**完全一致**（事件重放幂等），须有测试用例验证
- 并发读写通过 `go test -race` 零告警
- 任一 API 异常均返回结构化错误，不得 panic 导致进程退出（recover 中间件）

### 6.3 可观测性（全局记忆规则 2 强制）

- 后端提供统一 `pkg/logger`，支持 `debug/info/warn/error` 级别，通过 `LOG_LEVEL` 环境变量控制
- 前端提供统一 logger 工具，生产构建自动屏蔽 debug 输出
- **禁止**散落的 `fmt.Println` 与 `console.log`

### 6.4 测试覆盖（全局记忆规则 4 强制）

| 范围 | 要求 |
|---|---|
| 图引擎单元测试 | 核心算法（BFS/DFS/Dijkstra/全路径/连通分量）覆盖率 ≥ 80% |
| 事件溯源单元测试 | 重放一致性、检查点、时间点快照必测 |
| API 层测试 | 节点/边 CRUD + 核心查询端点 smoke 测试 |
| E2E | Playwright 覆盖：加载图谱 → 点击节点高亮 → 编辑属性 → 最短路径查询 → 时间轴回溯 |
| 成本 | 全部测试离线运行，预期花费 **¥0**（无外部计费 API） |

### 6.5 安全

- 全部 SQL 使用参数化查询（pgx 原生占位符），禁止字符串拼接
- 输入校验：所有 API 入参在 handler 层校验（长度、枚举合法性、权重正数、UUID 格式）
- 动态属性键名白名单正则 `^[a-zA-Z0-9_\-\u4e00-\u9fa5]{1,64}$`，防止 JSONB 注入异常键
- 全路径枚举与遍历强制上限（最大深度 10、最大路径数 1000、最大返回节点 20000），防止算法层 DoS
- CORS 白名单配置化，不使用 `*`

### 6.6 数据完整性（全局记忆规则 1 强制）

- 种子数据导入 / 事件反序列化必须校验结构完整性：字段存在性、类型、枚举合法性、边界值
- 反序列化失败的事件记录明确错误日志并中止重放（宁可启动失败，不可静默构建错误拓扑）

---

## 7. 交付物清单（Deliverables）

| 类别 | 交付物 |
|---|---|
| 代码 | `backend/`（Go，25–35 文件，5500–8000 行）、`frontend-user/`（Vue 3） |
| 容器 | `docker-compose.yml`、`backend/Dockerfile`（多阶段）、`frontend-user/Dockerfile`（多阶段 + Nginx） |
| 文档 | `docs/Requirements.md`（本文）、`docs/Roadmap.md`、`docs/DesignSpec.md`、`docs/API.md`、`docs/QA_Record.md`、`docs/AuditReport.md`、`docs/SelfTestReport.md`、`README.md` |
| 测试 | `backend/internal/**/*_test.go`、`tests/e2e_flow.spec.ts` |
| 数据 | 种子数据生成器（IT 资产场景 + 反欺诈场景两套示例图） |

---

## 8. 明确排除项（Out of Scope）

以下内容**明确不做**，防止范围漂移（Redline 4）：

1. 用户注册体系与 RBAC 权限管理（仅提供演示登录态或免登录）
2. 外部图数据库（Neo4j / NebulaGraph / JanusGraph）实际接入
3. 分布式 / 集群部署、图分片
4. 实时数据采集 Agent（CMDB 自动发现）
5. 图机器学习（社区发现算法、节点嵌入、GNN）
6. 移动端原生 App
7. 多租户隔离

---

## 9. 风险登记（Risk Register）

| # | 风险 | 影响 | 缓解措施 |
|---|---|---|---|
| R1 | 10 万节点图在前端 G6 画布直接渲染必然卡死 | 高 | 前端**永不**全量渲染。默认加载「核心视图」（≤ 500 节点），其余通过按需展开邻居获取；后端全量拓扑接口强制上限 |
| R2 | 事件日志无限增长导致冷启动重放变慢 | 中 | FR-B6 快照检查点机制 |
| R3 | 内存图与 DB 事件日志状态不一致 | 高 | FR-B1 先写日志后改内存 + 事务；启动重放作为唯一权威构建路径 |
| R4 | Dijkstra 在超大图上超时 | 中 | 引入访问节点数上限与超时 context，超限返回明确的「查询过宽」错误而非挂起 |
| R5 | 5500–8000 行代码量在单阶段内一次性生成，质量不可控 | 高 | Roadmap 强制分期（MVP / V1 / V2），每期独立可运行、可验证 |
| R6 | Docker 多架构镜像拉取失败 | 低 | 全部基础镜像选用官方 alpine 变体，Phase 1 验证 ARM64 可拉取 |

---

## 10. 分期边界建议（供 Phase 1 Roadmap 细化）

> 依据判据 3，本项目必须分期。PM 给出边界建议，Chief Architect 在 `docs/Roadmap.md` 中细化落地。

| 期 | 范围 | 完成定义（DoD） |
|---|---|---|
| **MVP** | 图引擎（邻接表 + BFS/DFS/Dijkstra）、节点/边 CRUD API、事件日志写入与启动重放、前端 G6 基础画布 + 拖拽缩放 + 邻居高亮、Docker 三容器可跑 | `docker compose up` 后可在浏览器看到图谱、增删节点、点击高亮 |
| **V1** | 最短路径高亮、全路径枚举、上下游血缘分析、动态属性抽屉编辑器、时间轴回溯与快照重建、统一日志与错误码 | 完整覆盖 Prompt 全部显式功能点 |
| **V2** | 拓扑 Diff 对比视图、快照检查点、邻接矩阵子图分析、二级索引优化、性能基线达标、E2E 测试全绿、美学打磨 | 通过 QA Phase 4 与 Auditor Phase 5 |

---

## 11. 验收签署

| 项 | 状态 |
|---|---|
| 废除评估 | ✅ ACCEPT |
| 矛盾裁决 | ✅ 5 项已冻结（C-1 ~ C-5） |
| Docker 交付标准 | ✅ 兼容 |
| 需求冻结 | ✅ 已冻结于 2026-08-21 20:05 (GMT+8) |
| 下一步 | 等待用户输入 `/auto` 启动 Auto-Swarm |
