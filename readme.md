# AOI 系统实现

一个基于 Go 语言实现的 AOI（Area of Interest，兴趣区域）系统，支持 2D 和 3D 场景，用于处理游戏或类似场景中实体间的视野感知关系。

## 核心功能

- 支持 2D 九宫格法和 3D 十字链表法两种 AOI 实现
- 实体位置管理与移动追踪
- 视野范围内实体的自动感知与 `Enter/Leave` 事件通知
- 灵活的视野订阅机制（玩家可订阅其他实体的视野变化）
- 可视化演示界面（基于 WebSocket + 前端渲染）

## 实现说明

### 2D 实现（`2d/` 目录）：九宫格算法
九宫格算法是 2D 场景下最经典、性能最优的 AOI 实现方式之一，核心思想是通过**空间分块**减少无效的实体遍历，以下是详细实现逻辑：

#### 1. 空间划分
- 将整个 2D 地图按照固定尺寸（如 100x100 像素）划分为若干个矩形网格（Grid），每个网格有唯一的二维索引 `(gridX, gridY)`。
- 网格尺寸需结合游戏视野半径设计（建议网格尺寸 = 视野半径），确保单个实体的视野范围最多覆盖周边 9 个网格（自身所在网格 + 8 个相邻网格）。

#### 2. 实体管理
- 每个网格维护一个实体集合（Set[EntityID]），记录当前位于该网格内的所有实体。
- 实体（Entity）核心属性：`ID`、`Position`（X/Y 坐标）、`ViewRange`（视野半径）。
- 实体添加/移动时，通过坐标计算所属网格索引：
  ```go
  gridX := int(entity.Pos.X / gridSize)
  gridY := int(entity.Pos.Y / gridSize)
  ```
  并将实体从原网格移除，添加至新网格。

#### 3. 视野计算
- 当需要获取某玩家的视野内实体时，先计算玩家所在网格，再遍历该网格周边的 9 个网格（九宫格）。
- 收集 9 个网格内的所有实体，得到视野内实体列表。


### 3D 实现（`3d/` 目录）：十字链表算法
十字链表（也叫轴标记链表）是 3D 场景下的高效 AOI 实现方式，核心思想是通过**轴分离**和**标记排序**，将 3D 空间的可见性判断拆解为三个轴的区间重叠判断，以下是详细实现逻辑：

#### 1. 轴标记设计
为每个实体在 X、Y、Z 三个轴上分别创建 3 种标记（Marker），所有标记按坐标值排序存储在对应轴的链表中：
- `PosMarker`：实体在该轴的实际坐标（如 X 轴的 `entity.Pos.X`）。
- `MinMarker`：实体视野范围的左/下/近边界（如 X 轴的 `entity.Pos.X - entity.ViewRange`）。
- `MaxMarker`：实体视野范围的右/上/远边界（如 X 轴的 `entity.Pos.X + entity.ViewRange`）。

#### 2. 可见性判断逻辑
两个实体 A 和 B 互相可见的充要条件是：**三个轴的视野区间均重叠**，即：
```
A.MinX ≤ B.PosX ≤ A.MaxX 
且 A.MinY ≤ B.PosY ≤ A.MaxY 
且 A.MinZ ≤ B.PosZ ≤ A.MaxZ
```
十字链表通过以下方式高效验证该条件：
- 对每个轴的链表，遍历某实体的 `MinMarker` 到 `MaxMarker` 之间的所有 `PosMarker`，记录这些标记对应的实体。
- 三个轴的交集即为该实体的视野内实体。

#### 3. 实体移动处理
- 实体移动时，先从 X/Y/Z 三轴的链表中移除旧的 3 类标记。
- 根据新坐标计算新的 `Min/Max/Pos` 标记，插入到对应轴的链表中（保持链表有序）。
- 触发视野重算：通知所有与该实体视野重叠的实体，更新可见性状态。


### 核心机制：Enter/Leave 事件
`Enter/Leave` 事件是 AOI 系统的核心交互能力，用于在实体进入/离开玩家视野时触发自定义逻辑（如游戏内的角色显隐、音效播放、战斗检测等）。

#### 1. 事件定义
系统通过 `AOICallback` 接口定义事件回调：
```go
// AOICallback 视野变化回调接口
type AOICallback interface {
    OnEnter(watcher PlayerID, target EntityID)  // 目标实体进入观察者视野
    OnLeave(watcher PlayerID, target EntityID)  // 目标实体离开观察者视野
}
```

#### 2. 事件触发时机
- **OnEnter**：
  - 新实体添加到地图且落入某玩家视野范围；
  - 实体移动后首次进入某玩家视野；
  - 玩家移动后首次包含某实体到自身视野。
- **OnLeave**：
  - 实体被移除且当前在某玩家视野内；
  - 实体移动后离开某玩家视野；
  - 玩家移动后不再包含某实体到自身视野；
  - 实体/玩家的视野半径调整导致视野范围变化。

#### 3. 事件实现逻辑
- 系统维护每个玩家的「当前视野实体集合」；
- 每次视野重算后，对比「新视野集合」与「旧视野集合」：
  - 新增的实体触发 `OnEnter`；
  - 消失的实体触发 `OnLeave`；
- 事件触发完成后，更新「当前视野实体集合」。

### 核心机制：视野订阅机制
订阅机制允许玩家（观察者）主动关注特定实体的视野变化，实现精细化的视野管理（如仅监听队友、BOSS、敌对玩家的视野状态）。

#### 1. 订阅规则
- 订阅方向：玩家（Subscriber）→ 目标实体（Target）；
- 一对多：单个玩家可订阅多个实体，单个实体可被多个玩家订阅；
- 订阅生效：仅当目标实体进入/离开订阅者视野时，订阅者才会收到 `Enter/Leave` 事件；
- 取消订阅：停止接收目标实体的视野变化事件。

#### 2. 核心接口与实现
```go
// 订阅目标实体的视野变化
func (m *AOIManager) Subscribe(subscriber PlayerID, target EntityID) {
    // 维护订阅映射表：subscriber -> 订阅的实体集合
    m.subMap[subscriber].Add(target)
    // 立即检查当前是否可见，若可见则触发 OnEnter
    if m.CanSee(subscriber, target) {
        m.callback.OnEnter(subscriber, target)
    }
}

// 取消订阅目标实体
func (m *AOIManager) Unsubscribe(subscriber PlayerID, target EntityID) {
    // 从订阅映射表中移除
    m.subMap[subscriber].Remove(target)
    // 若当前可见，触发 OnLeave
    if m.CanSee(subscriber, target) {
        m.callback.OnLeave(subscriber, target)
    }
}
```

#### 3. 订阅机制的优化价值
- **性能优化**：无需为玩家推送全量视野事件，仅推送订阅实体的状态变化；
- **业务适配**：
  - 游戏内「关注列表」功能：仅监听关注的玩家；
  - 「组队系统」：仅订阅队友的视野状态；
  - 「任务系统」：仅订阅任务目标实体的视野状态；
- **灵活扩展**：支持批量订阅/取消订阅（如订阅整个队伍、整个公会的实体）。

## 快速开始

### 依赖安装
```bash
go mod tidy
```

### 运行演示
#### 2D 演示
```bash
cd 2d
go test
```
访问 `http://localhost:8080` 查看 2D 可视化界面

#### 3D 演示
```bash
cd 3d
go test
```
访问 `http://localhost:8081` 查看 3D 可视化界面（使用 Three.js 渲染）

## 可视化操作

### 2D 演示
- 点击或拖拽鼠标移动蓝色主角
- 绿色点表示视野源（眼），红色点表示普通实体
- 线条展示不同类型的视野范围和感知关系
- 控制台可查看 `Enter/Leave` 事件日志和订阅状态变化

### 3D 演示
- 使用 `W/A/S/D` 键进行平面移动（X/Z 轴）
- 使用 `R`/`F` 键进行上下移动（Y 轴）
- 鼠标拖拽可旋转视角
- 绿色实体表示在视野范围内，红色表示不在视野范围外
- 点击实体可订阅/取消订阅，订阅后实体旁会显示「订阅标记」

## 代码结构
```
aoi/
├── 2d/                # 2D AOI 实现
│   ├── aoi.go         # 九宫格核心逻辑（Grid/GridManager/Entity）
│   ├── aoi_test.go    # 测试与演示服务
│   └── static/        # 2D 可视化前端
├── 3d/                # 3D AOI 实现
│   ├── aoi.go         # 十字链表核心逻辑（Marker/AxisList/3DManager）
│   ├── aoi_test.go    # 测试与演示服务
│   └── static/        # 3D 可视化前端（Three.js）
├── aoi_interface.go   # 通用接口定义（含 AOICallback）
├── set.go             # 集合工具类（用于视野/订阅集合管理）
├── go.mod             # 依赖管理
└── go.sum             # 依赖校验
```

## 核心接口
```go
// AOIManager 定义了 AOI 系统的核心接口
type AOIManager interface {
    AddPlayer(id PlayerID)                      // 添加玩家
    AddEntity(id EntityID, pos *Position, rangeVal Float)  // 添加实体
    RemoveEntity(id EntityID)                   // 移除实体
    MoveEntity(id EntityID, pos *Position)      // 移动实体
    GetView(id PlayerID) Set[EntityID]          // 获取视野内实体
    CanSee(watcherId PlayerID, targetId EntityID) bool  // 检查是否可见
    Subscribe(subscriber PlayerID, target EntityID)     // 订阅视野
    Unsubscribe(subscriber PlayerID, target EntityID)   // 取消订阅
    SetCallback(cb AOICallback)                 // 设置视野变化回调
}

// AOICallback 视野变化回调接口
type AOICallback interface {
    OnEnter(watcher PlayerID, target EntityID)  // 实体进入视野
    OnLeave(watcher PlayerID, target EntityID)  // 实体离开视野
}
```

## 适用场景
- 2D 九宫格：2D 游戏视野管理、地图怪物感知、玩家交互检测。
- 3D 十字链表：3D 游戏角色视野、虚拟场景实体交互、VR/AR 空间感知。
- 订阅/事件机制：游戏关注列表、组队视野同步、任务目标追踪、动态场景加载。