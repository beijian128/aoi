package three_dim

import (
	"math"
)

// ==========================================
// 1. 基础数据结构定义
// ==========================================

// MarkerType 节点类型
type MarkerType int

const (
	MarkerMin MarkerType = 0 // 视野下界 (Watcher's View Start)
	MarkerMax MarkerType = 1 // 视野上界 (Watcher's View End)
	MarkerPos MarkerType = 2 // 实体位置 (Target's Body)
)

// Marker 链表节点
type Marker struct {
	Type  MarkerType
	Axis  int // 0:X, 1:Y, 2:Z
	Val   float64
	Owner *Entity

	prev *Marker
	next *Marker
}

// AxisList 双向链表
type AxisList struct {
	Head *Marker // -Inf
	Tail *Marker // +Inf
}

// Player 玩家/阵营 (逻辑层)
// 它是视野的订阅者，本身没有坐标，汇总其下属 Entity 的视野
type Player struct {
	ID int64
	// FinalView: 聚合后的视野
	// Key: TargetID, Value: 引用计数 (有多少个我的单位看见了这个目标)
	FinalView map[int64]int
}

// Entity 物理实体 (物理层)
// 它是视野的提供者，也是被观察的对象
type Entity struct {
	ID    int64
	Pos   [3]float64
	Range float64 // 视野半径 (立方体半边长)

	// 链表节点: [3个轴][3种类型]
	Markers [3][3]*Marker

	// ViewCounts: 物理视野计数器
	// Key: TargetID
	// Value: 轴匹配数 (0-3). 当且仅当 == 3 时，物理上可见
	ViewCounts map[int64]int

	// VisibleSet: 当前物理上真正看见的集合 (ViewCounts==3 的子集)
	VisibleSet map[int64]bool

	// Subscribers: 哪些玩家订阅了我的视野
	// Key: PlayerID
	Subscribers map[int64]*Player
}

// Callback 回调接口
type AOICallback interface {
	// 玩家视野变化回调 (去重后的结果)
	OnPlayerEnter(playerID, targetID int64)
	OnPlayerLeave(playerID, targetID int64)
}

// Manager AOI 管理器
type Manager struct {
	Axes     [3]*AxisList
	Entities map[int64]*Entity
	Players  map[int64]*Player
	Callback AOICallback
}

// ==========================================
// 2. Manager 核心逻辑
// ==========================================

func NewManager() *Manager {
	m := &Manager{
		Entities: make(map[int64]*Entity),
		Players:  make(map[int64]*Player),
	}
	// 初始化三轴链表哨兵
	for i := 0; i < 3; i++ {
		head := &Marker{Val: math.Inf(-1)}
		tail := &Marker{Val: math.Inf(1)}
		head.next = tail
		tail.prev = head
		m.Axes[i] = &AxisList{Head: head, Tail: tail}
	}
	return m
}

func (m *Manager) SetCallback(cb AOICallback) {
	m.Callback = cb
}

// AddPlayer 注册玩家
func (m *Manager) AddPlayer(id int64) {
	if _, ok := m.Players[id]; !ok {
		m.Players[id] = &Player{
			ID:        id,
			FinalView: make(map[int64]int),
		}
	}
}

// AddEntity 添加物理单位
func (m *Manager) AddEntity(id int64, x, y, z, rangeVal float64) {
	if _, ok := m.Entities[id]; ok {
		return
	}

	e := &Entity{
		ID:          id,
		Pos:         [3]float64{x, y, z},
		Range:       rangeVal,
		ViewCounts:  make(map[int64]int),
		VisibleSet:  make(map[int64]bool),
		Subscribers: make(map[int64]*Player),
	}

	// 创建并链接节点
	vals := [3]float64{x, y, z}
	for axis := 0; axis < 3; axis++ {
		// 创建
		e.Markers[axis][MarkerMin] = &Marker{Type: MarkerMin, Axis: axis, Val: vals[axis] - rangeVal, Owner: e}
		e.Markers[axis][MarkerMax] = &Marker{Type: MarkerMax, Axis: axis, Val: vals[axis] + rangeVal, Owner: e}
		e.Markers[axis][MarkerPos] = &Marker{Type: MarkerPos, Axis: axis, Val: vals[axis], Owner: e}

		// 简单插入到尾部前 (依靠后面的 Update 进行排序)
		list := m.Axes[axis]
		prev := list.Tail.prev

		// 链接 Min
		prev.next = e.Markers[axis][MarkerMin]
		e.Markers[axis][MarkerMin].prev = prev
		// 链接 Pos
		e.Markers[axis][MarkerMin].next = e.Markers[axis][MarkerPos]
		e.Markers[axis][MarkerPos].prev = e.Markers[axis][MarkerMin]
		// 链接 Max
		e.Markers[axis][MarkerPos].next = e.Markers[axis][MarkerMax]
		e.Markers[axis][MarkerMax].prev = e.Markers[axis][MarkerPos]
		// 链接 Tail
		e.Markers[axis][MarkerMax].next = list.Tail
		list.Tail.prev = e.Markers[axis][MarkerMax]
	}

	m.Entities[id] = e

	// 立即更新位置以触发正确的排序和AOI计算
	m.updateEntity(e, x, y, z)
}

// RemoveEntity 移除物理单位
func (m *Manager) RemoveEntity(id int64) {
	e, ok := m.Entities[id]
	if !ok {
		return
	}

	// 1. 触发该单位视野的丢失 (通知订阅者)
	for targetID := range e.VisibleSet {
		m.notifySubscribers(e, targetID, false)
	}

	m.updateEntity(e, 999999, 999999, 999999)

	// 3. 物理断开
	for axis := 0; axis < 3; axis++ {
		for typeIdx := 0; typeIdx < 3; typeIdx++ {
			node := e.Markers[axis][typeIdx]
			node.prev.next = node.next
			node.next.prev = node.prev
		}
	}
	delete(m.Entities, id)
}

// Move 移动
func (m *Manager) Move(id int64, x, y, z float64) {
	if e, ok := m.Entities[id]; ok {
		m.updateEntity(e, x, y, z)
	}
}

// Subscribe 视野订阅 (插眼)
func (m *Manager) Subscribe(playerID, entityID int64) {
	p, pok := m.Players[playerID]
	e, eok := m.Entities[entityID]
	if !pok || !eok {
		return
	}

	if _, exists := e.Subscribers[playerID]; exists {
		return
	}

	// 建立关系
	e.Subscribers[playerID] = p

	// 立即同步当前视野
	for targetID := range e.VisibleSet {
		m.refCountChange(p, targetID, 1)
	}
}

// Unsubscribe 取消订阅 (眼消失)
func (m *Manager) Unsubscribe(playerID, entityID int64) {
	p, pok := m.Players[playerID]
	e, eok := m.Entities[entityID]
	if !pok || !eok {
		return
	}

	if _, exists := e.Subscribers[playerID]; !exists {
		return
	}

	// 解除关系
	delete(e.Subscribers, playerID)

	// 立即移除贡献
	for targetID := range e.VisibleSet {
		m.refCountChange(p, targetID, -1)
	}
}

// GetPlayerView 获取玩家当前能看到的所有目标
func (m *Manager) GetPlayerView(playerID int64) []int64 {
	p, ok := m.Players[playerID]
	if !ok {
		return nil
	}

	res := make([]int64, 0, len(p.FinalView))
	for tid := range p.FinalView {
		res = append(res, tid)
	}
	return res
}

// ==========================================
// 3. 核心算法实现
// ==========================================

func (m *Manager) updateEntity(e *Entity, x, y, z float64) {
	e.Pos = [3]float64{x, y, z}
	newVals := [3]float64{x, y, z}

	for axis := 0; axis < 3; axis++ {
		// 依次更新 Min, Max, Pos
		// 注意：更新顺序不影响最终正确性，因为 Swap 会处理一切
		m.updateMarker(e.Markers[axis][MarkerMin], newVals[axis]-e.Range)
		m.updateMarker(e.Markers[axis][MarkerMax], newVals[axis]+e.Range)
		m.updateMarker(e.Markers[axis][MarkerPos], newVals[axis])
	}
}

func (m *Manager) updateMarker(node *Marker, newVal float64) {
	node.Val = newVal

	// 向右移动 (Val 变大)
	for node.next != nil && !math.IsInf(node.next.Val, 0) && node.Val > node.next.Val {
		other := node.next
		m.swap(node, other) // node 换到 other 后面
		m.checkCross(node, other, true)
	}
	// 向左移动 (Val 变小)
	for node.prev != nil && !math.IsInf(node.prev.Val, 0) && node.Val < node.prev.Val {
		other := node.prev
		m.swap(other, node) // other 换到 node 后面 (即 node 换到 other 前面)
		m.checkCross(node, other, false)
	}
}

// swap 交换相邻节点: left -> right ==> right -> left
func (m *Manager) swap(left, right *Marker) {
	left.prev.next = right
	right.prev = left.prev
	right.next.prev = left
	left.next = right.next
	right.next = left
	left.prev = right
}

// checkCross 核心穿透逻辑
// mover: 正在移动的节点
// passive: 被越过的节点
// movingRight: mover 的移动方向
func (m *Manager) checkCross(mover, passive *Marker, movingRight bool) {
	if mover.Owner == passive.Owner {
		return
	} // 忽略自己

	// 识别谁是 Watcher (Min/Max)，谁是 Target (Pos)
	var watcherNode, targetNode *Marker

	if (mover.Type == MarkerMin || mover.Type == MarkerMax) && passive.Type == MarkerPos {
		watcherNode = mover
		targetNode = passive
	} else if mover.Type == MarkerPos && (passive.Type == MarkerMin || passive.Type == MarkerMax) {
		watcherNode = passive
		targetNode = mover
	} else {
		// 边界穿边界，或 Pos 穿 Pos，不影响可见性
		return
	}

	watcher := watcherNode.Owner
	target := targetNode.Owner

	// 判定是进入视野(Enter) 还是 离开视野(Leave)
	// 逻辑矩阵：
	// Min 向右过 Pos -> Leave (Range Shrink)
	// Min 向左过 Pos -> Enter (Range Expand)
	// Max 向右过 Pos -> Enter (Range Expand)
	// Max 向左过 Pos -> Leave (Range Shrink)
	// Pos 向右过 Min -> Enter (Target Enter)
	// Pos 向左过 Min -> Leave (Target Leave)
	// Pos 向右过 Max -> Leave (Target Leave)
	// Pos 向左过 Max -> Enter (Target Enter)

	isEnter := false

	// 简化判断逻辑：
	// 我们只要判断 swap 发生**后**，Pos 是否在 [Min, Max] 区间内？
	// 但由于我们是增量更新，我们必须知道这是"变好"还是"变坏"。
	// 采用上述矩阵：

	if watcherNode.Type == MarkerMin {
		// Watcher Min vs Target Pos
		if mover == watcherNode { // Min 动
			isEnter = !movingRight // Min 往左是 Enter
		} else { // Pos 动
			isEnter = movingRight // Pos 往右是 Enter
		}
	} else {
		// Watcher Max vs Target Pos
		if mover == watcherNode { // Max 动
			isEnter = movingRight // Max 往右是 Enter
		} else { // Pos 动
			isEnter = !movingRight // Pos 往左是 Enter
		}
	}

	// 更新物理计数
	delta := -1
	if isEnter {
		delta = 1
	}

	oldC := watcher.ViewCounts[target.ID]
	newC := oldC + delta

	// 清理 map 防止内存泄漏
	if newC <= 0 {
		delete(watcher.ViewCounts, target.ID)
		newC = 0 // 修正为0以防逻辑错误
	} else {
		watcher.ViewCounts[target.ID] = newC
	}

	// 状态机翻转 (3轴全中)
	if oldC < 3 && newC == 3 {
		// 物理 Enter
		watcher.VisibleSet[target.ID] = true
		m.notifySubscribers(watcher, target.ID, true)
	} else if oldC == 3 && newC < 3 {
		// 物理 Leave
		delete(watcher.VisibleSet, target.ID)
		m.notifySubscribers(watcher, target.ID, false)
	}
}

// notifySubscribers 通知所有订阅者
func (m *Manager) notifySubscribers(source *Entity, targetID int64, isEnter bool) {
	delta := -1
	if isEnter {
		delta = 1
	}

	for _, player := range source.Subscribers {
		m.refCountChange(player, targetID, delta)
	}
}

// refCountChange 玩家引用计数变更
func (m *Manager) refCountChange(p *Player, targetID int64, delta int) {
	oldVal := p.FinalView[targetID]
	newVal := oldVal + delta

	if newVal <= 0 {
		delete(p.FinalView, targetID)
	} else {
		p.FinalView[targetID] = newVal
	}

	// 触发回调 (0 -> 1 Enter, 1 -> 0 Leave)
	if m.Callback != nil {
		if oldVal == 0 && newVal > 0 {
			m.Callback.OnPlayerEnter(p.ID, targetID)
		} else if oldVal > 0 && newVal <= 0 {
			m.Callback.OnPlayerLeave(p.ID, targetID)
		}
	}
}
