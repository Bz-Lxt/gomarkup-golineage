# GoLineage — 交付路线图（Roadmap）

| 项 | 值 |
|---|---|
| 文档版本 | v1.0 |
| 生成时间 | 2026-08-21 20:15 (GMT+8) |
| 负责阶段 | Phase 1 — Chief Architect (Alkaid-SOP v13.0) |
| 上游 SSOT | `docs/Requirements.md` |

> 本文件定义 **WHEN**（何时做）。范围以 `docs/Requirements.md` 为准，本文件不得新增需求。

---

## 0. 🔀 阶段顺序决策（Phase Order Decision）

### 裁决：**Logic-First（交换 Phase 2 与 Phase 3）**

**一句话理由**：本项目前端是关系图谱**画布（canvas）**，其组件结构、G6 节点/边样式映射、抽屉表单字段、时间轴数据绑定，全部**派生自后端的图数据模型与算法返回结构**（`Node` / `Edge` / `PathResult` / `LineageEvent` / `TopologySnapshot`）；在数据契约未落地前构建 UI 必然返工。

**执行顺序**：`Phase 1 架构` → `Phase 3 逻辑层` → `Phase 2 UI 层` → `Phase 4 QA` → `Phase 5 审计`

---

## 1. 端口分配（开发期随机端口）

依据 DevOps 记忆「Dev use random(10000-60000)，避开 1848x 段」，已探测确认空闲：

| 服务 | 容器名 | 宿主机端口 | 容器内端口 |
|---|---|---|---|
| 前端（Nginx） | `golineage-web` | **27430** | 80 |
| 后端（Go API） | `golineage-api` | **27431** | 8080 |
| 数据库（PostgreSQL 17） | `golineage-db` | **27432** | 5432 |

> `/deploy` 阶段将统一改写为标准 8081+ 段。

---

## 2. 目录结构（已落地）

```
GoLineage/
├── backend/                        # Go 后端
│   ├── cmd/server/                 # 程序入口
│   ├── internal/
│   │   ├── config/                 # 环境配置加载
│   │   ├── graph/                  # ★ 手写内存图引擎（核心考核点）
│   │   ├── eventstore/             # ★ 事件溯源与时间轴回溯
│   │   ├── repository/             # GraphRepository 接口（图数据库适配 Port）
│   │   ├── service/                # 业务编排层
│   │   ├── api/                    # HTTP handler / router / DTO / 中间件
│   │   ├── seed/                   # 两套场景种子数据生成器
│   │   └── timeutil/               # GMT+8 时间工具
│   ├── pkg/logger/                 # 统一分级日志
│   ├── migrations/                 # SQL 迁移脚本
│   └── Dockerfile                  # 多阶段构建
├── frontend-user/                  # Vue 3 + G6 前端
│   ├── src/{api,components,composables,stores,views,styles,types,utils}
│   └── Dockerfile                  # 多阶段构建 + Nginx
├── tests/                          # Playwright E2E
├── docs/                           # 全部交付文档
└── docker-compose.yml
```

---

## 3. 分期计划

### 🥇 MVP — 可运行的最小图谱系统

**完成定义（DoD）**：`docker compose up --build -d` 后浏览器可见图谱、能增删节点与边、点击节点高亮邻居、重启数据不丢。

| # | 任务 | 需求映射 | 状态 |
|---|---|---|---|
| M-01 | Go module 初始化、配置加载、GMT+8 时间工具、统一 Logger | NFR 6.3 | [ ] |
| M-02 | 图引擎：Node/Edge 领域模型 + 邻接表结构 + 并发安全增删改 | FR-A1, FR-A2 | [ ] |
| M-03 | 图引擎：BFS / DFS 遍历（深度、方向、关系类型过滤 + 环检测） | FR-A3, FR-A4 | [ ] |
| M-04 | 图引擎：手写二叉堆 + Dijkstra 最短路径 | FR-A5 | [ ] |
| M-05 | 事件模型 + PostgreSQL 事件日志表 + 迁移脚本 | FR-B1 | [ ] |
| M-06 | 写路径：事务内先写事件后改内存图 | FR-B1 | [ ] |
| M-07 | 启动时全量重放重建内存图 | FR-B5 | [ ] |
| M-08 | 节点 / 边 CRUD REST API + 统一响应包络 + 错误码 | FR-C1, FR-C2, FR-C7 | [ ] |
| M-09 | 邻居子图 API + 全量拓扑 API（带上限保护） | FR-A7, FR-C4 | [ ] |
| M-10 | 健康检查 + recover / CORS / trace_id 中间件 | FR-C6, NFR 6.2 | [ ] |
| M-11 | 种子数据生成器（IT 资产 + 反欺诈两套场景） | §7 | [ ] |
| M-12 | 后端 Dockerfile（多阶段 + GOPROXY 国内镜像） | Redline 1 | [ ] |
| M-13 | Vue 3 + Vite + TS + Tailwind + Element Plus 工程骨架 | §3 | [ ] |
| M-14 | G6 画布渲染 + 拖拽 + 缩放 + 平移 + 适应画布 | FR-D1, FR-D2 | [ ] |
| M-15 | 点击节点高亮一跳邻居（无关元素淡出） | FR-D3 | [ ] |
| M-16 | 前端 Dockerfile（多阶段镜像内构建 + Nginx 反代） | Redline 1 | [ ] |
| M-17 | docker-compose 三容器编排 + 健康检查 + TZ | Redline 1 | [ ] |

### 🥈 V1 — 覆盖 Prompt 全部显式功能点

**完成定义（DoD）**：Prompt 中所有显式要求的功能均可在浏览器完成操作闭环。

| # | 任务 | 需求映射 | 状态 |
|---|---|---|---|
| V-01 | 图引擎：全路径枚举（深度/条数上限保护） | FR-A6 | [ ] |
| V-02 | 图引擎：上下游血缘有向可达分析 | FR-A8 | [ ] |
| V-03 | 图引擎：二级索引（type / name 前缀 / 属性键值） | FR-A9 | [ ] |
| V-04 | 时间轴：事件流水分页查询 API | FR-B2 | [ ] |
| V-05 | 时间轴：任意时刻历史拓扑重建 API | FR-B3 | [ ] |
| V-06 | 最短路径 API + 前端起终点选择与路径流动高亮 | FR-D4 | [ ] |
| V-07 | 布局切换（force / dagre / radial） | FR-D5 | [ ] |
| V-08 | 节点搜索与画布聚焦 | FR-D6 | [ ] |
| V-09 | 右侧抽屉：动态属性增删改 + 类型预设模板 | FR-E1~E3 | [ ] |
| V-10 | 风险等级语义渲染（标签 + 节点描边） | FR-E4 | [ ] |
| V-11 | 抽屉内实体变更历史时间线 | FR-E5 | [ ] |
| V-12 | 时间轴回溯 UI（滑块 + 实时/历史模式切换） | FR-F1, FR-F2 | [ ] |
| V-13 | 变更流水面板（含 before/after diff） | FR-F3 | [ ] |
| V-14 | `docs/API.md` 完整接口文档（示例 + 错误码表） | FR-C8 | [ ] |

### 🥉 V2 — 工程质量与验收达标

**完成定义（DoD）**：通过 Phase 4 QA 与 Phase 5 审计。

| # | 任务 | 需求映射 | 状态 |
|---|---|---|---|
| W-01 | 拓扑 Diff API + 前端两时间点对比视图 | FR-B4, FR-F4 | [ ] |
| W-02 | 快照检查点机制（重放加速） | FR-B6 | [ ] |
| W-03 | 邻接矩阵子图分析（连通分量 / 稠密度，≤500 节点） | FR-A10 | [ ] |
| W-04 | 图引擎单元测试覆盖率 ≥ 80% + `-race` 零告警 | NFR 6.4, 6.2 | [ ] |
| W-05 | 事件重放一致性 / 检查点 / 时间点快照测试 | NFR 6.4 | [ ] |
| W-06 | API 层 smoke 测试 | NFR 6.4 | [ ] |
| W-07 | 性能基准测试（benchmark）与基线记录 | NFR 6.1 | [ ] |
| W-08 | Playwright E2E 全流程 | NFR 6.4 | [ ] |
| W-09 | 美学打磨：空状态 / 骨架屏 / 错误态 / 响应式 | FR-G | [ ] |
| W-10 | 安全加固：参数化查询、输入校验、算法上限、CORS 白名单 | NFR 6.5 | [ ] |

---

## 4. 关键架构决策记录（ADR）

### ADR-1：CQRS + 事件溯源

```
写路径:  HTTP → Service → [BEGIN TX] → 事件日志 INSERT → COMMIT → 应用到内存图
读路径:  HTTP → Service → 内存图（邻接表）→ BFS/DFS/Dijkstra → DTO
回溯:    HTTP → Replay(events ≤ T) → 独立内存图实例 → 拓扑快照
恢复:    进程启动 → 载入最近检查点 → 重放增量事件 → 内存图就绪
```

**不变式**：事件日志是唯一权威数据源；内存图是可随时丢弃重建的投影。

### ADR-2：邻接表双向索引

```go
type Graph struct {
    mu       sync.RWMutex
    nodes    map[NodeID]*Node
    outEdges map[NodeID]map[EdgeID]*Edge  // 出边：下游血缘
    inEdges  map[NodeID]map[EdgeID]*Edge  // 入边：上游血缘
    edges    map[EdgeID]*Edge
    idx      *Index                        // 二级索引
}
```

入边表的存在使「上游影响源分析」与「下游影响面分析」均为 O(度数)，无需全图扫描。

### ADR-3：分层依赖方向（严格单向，禁止反向引用）

```
api  →  service  →  repository(interface)  →  graph | eventstore
                                    ↑
                          MemoryGraphAdapter（唯一实现）
```

`graph` 包**零外部依赖**（仅标准库），保证图引擎可独立测试与复用。

### ADR-4：算法安全上限（防 DoS）

| 约束 | 默认值 | 可配置项 |
|---|---|---|
| 最大遍历深度 | 10 | `GRAPH_MAX_DEPTH` |
| 全路径最大条数 | 1000 | `GRAPH_MAX_PATHS` |
| 单次返回最大节点数 | 20000 | `GRAPH_MAX_NODES` |
| 邻接矩阵子图上限 | 500 | 硬编码常量 |
| 查询超时 | 10s | `GRAPH_QUERY_TIMEOUT` |

---

## 5. 风险应对（承接 Requirements §9）

| 风险 | 落地动作 | 归属 |
|---|---|---|
| R1 前端渲染爆炸 | 全量拓扑接口默认 `limit=500`；前端按需展开邻居 | M-09 / M-14 |
| R2 重放变慢 | 检查点机制 | W-02 |
| R3 内存与 DB 不一致 | 先写日志后改内存 + 启动重放为唯一构建路径 | M-06 / M-07 |
| R4 Dijkstra 超时 | context 超时 + 访问节点数上限 | ADR-4 |
| R5 代码量过大质量失控 | 三期交付，每期独立可运行 | 本文档 |
| R6 多架构镜像 | 全部 alpine 官方镜像；Dockerfile 内置国内源三件套 | M-12 / M-16 |

---

## 6. 进度总览

| 阶段 | 任务数 | 完成 | 状态 |
|---|---|---|---|
| MVP | 17 | 0 | ⏳ 待开始 |
| V1 | 14 | 0 | ⏳ 待开始 |
| V2 | 10 | 0 | ⏳ 待开始 |

> 由 Phase 3 / Phase 2 各 Agent 在完成任务时逐项回写勾选（SOP 8-Section 第 8 步 Sync）。
