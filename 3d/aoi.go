package three_dim

import (
	"github.com/beijian128/aoi"
)

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
	Val   aoi.Float
	Owner *Entity

	prev *Marker
	next *Marker
}

// AxisList 双向链表
type AxisList struct {
	Head *Marker // -Inf
	Tail *Marker // +Inf
}

// Entity 物理实体 (物理层)
// 它是视野的提供者，也是被观察的对象
type Entity struct {
	ID    aoi.EntityID
	Pos   [3]aoi.Float
	Range aoi.Float // 视野半径 (立方体半边长)

	// 链表节点: [3个轴][3种类型]
	Markers [3][3]*Marker

	// ViewCounts: 物理视野计数器
	// Key: TargetID
	// Value: 轴匹配数 (0-3). 当且仅当 == 3 时，物理上可见
	ViewCounts map[aoi.EntityID]int

	// VisibleSet: 当前物理上真正看见的集合 (ViewCounts==3 的子集)
	VisibleSet map[aoi.EntityID]bool

	// Subscribers: 哪些玩家订阅了我的视野
	// Key: PlayerID
	Subscribers map[aoi.PlayerID]*aoi.Player
}

// Manager AOI 管理器
type Manager struct {
	axes          [3]*AxisList
	entities      map[aoi.EntityID]*Entity
	players       map[aoi.PlayerID]*aoi.Player
	eventCallback aoi.AOICallback
}

func (m *Manager) CanSee(watcherId aoi.PlayerID, targetId aoi.EntityID) bool {
	watcher := m.players[watcherId]
	if watcher == nil {
		return false
	}
	return watcher.FinalView[targetId] > 0
}

func NewManager() *Manager {
	m := &Manager{
		entities: make(map[aoi.EntityID]*Entity),
		players:  make(map[aoi.PlayerID]*aoi.Player),
	}
	// 初始化三轴链表哨兵
	for i := 0; i < 3; i++ {
		head := &Marker{Val: aoi.FloatInf(-1)}
		tail := &Marker{Val: aoi.FloatInf(1)}
		head.next = tail
		tail.prev = head
		m.axes[i] = &AxisList{Head: head, Tail: tail}
	}
	return m
}

func (m *Manager) SetCallback(cb aoi.AOICallback) {
	m.eventCallback = cb
}

// AddPlayer 注册玩家
func (m *Manager) AddPlayer(id aoi.PlayerID) {
	if _, ok := m.players[id]; !ok {
		m.players[id] = &aoi.Player{
			ID:        id,
			FinalView: make(map[aoi.EntityID]int),
		}
	}
}

// AddEntity 添加物理单位
func (m *Manager) AddEntity(id aoi.EntityID, pos *aoi.Position, rangeVal aoi.Float) {
	if _, ok := m.entities[id]; ok {
		return
	}
	x, y, z := pos.X, pos.Y, pos.Z
	e := &Entity{
		ID:          id,
		Pos:         [3]aoi.Float{x, y, z},
		Range:       rangeVal,
		ViewCounts:  make(map[aoi.EntityID]int),
		VisibleSet:  make(map[aoi.EntityID]bool),
		Subscribers: make(map[aoi.PlayerID]*aoi.Player),
	}

	// 创建并链接节点
	vals := [3]aoi.Float{x, y, z}
	for axis := 0; axis < 3; axis++ {
		// 创建
		e.Markers[axis][MarkerMin] = &Marker{Type: MarkerMin, Axis: axis, Val: vals[axis] - rangeVal, Owner: e}
		e.Markers[axis][MarkerMax] = &Marker{Type: MarkerMax, Axis: axis, Val: vals[axis] + rangeVal, Owner: e}
		e.Markers[axis][MarkerPos] = &Marker{Type: MarkerPos, Axis: axis, Val: vals[axis], Owner: e}

		// 简单插入到尾部前 (依靠后面的 Update 进行排序)
		list := m.axes[axis]
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

	m.entities[id] = e

	// 立即更新位置以触发正确的排序和AOI计算
	m.updateEntity(e, x, y, z)
}

// RemoveEntity 移除物理单位
func (m *Manager) RemoveEntity(id aoi.EntityID) {
	e, ok := m.entities[id]
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
	delete(m.entities, id)
}

func (m *Manager) MoveEntity(id aoi.EntityID, pos *aoi.Position) {
	if e, ok := m.entities[id]; ok {
		m.updateEntity(e, pos.X, pos.Y, pos.Z)
	}
}

// Subscribe 视野订阅
func (m *Manager) Subscribe(playerID aoi.PlayerID, entityID aoi.EntityID) {
	p, pok := m.players[playerID]
	e, eok := m.entities[entityID]
	if !pok || !eok {
		return
	}

	if _, exists := e.Subscribers[playerID]; exists {
		return
	}

	e.Subscribers[playerID] = p

	// 立即同步当前视野
	for targetID := range e.VisibleSet {
		m.refCountChange(p, targetID, 1)
	}
}

// Unsubscribe 取消订阅
func (m *Manager) Unsubscribe(playerID aoi.PlayerID, entityID aoi.EntityID) {
	p, pok := m.players[playerID]
	e, eok := m.entities[entityID]
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

func (m *Manager) GetView(id aoi.PlayerID) aoi.Set[aoi.EntityID] {
	p, ok := m.players[id]
	if !ok {
		return nil
	}

	res := aoi.NewSet[aoi.EntityID]()
	for tid := range p.FinalView {
		res.Add(tid)
	}
	return res
}

func (m *Manager) updateEntity(e *Entity, x, y, z aoi.Float) {
	e.Pos = [3]aoi.Float{x, y, z}
	newVals := [3]aoi.Float{x, y, z}

	for axis := 0; axis < 3; axis++ {
		m.updateMarker(e.Markers[axis][MarkerMin], newVals[axis]-e.Range)
		m.updateMarker(e.Markers[axis][MarkerMax], newVals[axis]+e.Range)
		m.updateMarker(e.Markers[axis][MarkerPos], newVals[axis])
	}
}

func (m *Manager) updateMarker(node *Marker, newVal aoi.Float) {
	node.Val = newVal

	// 向右移动 (Val 变大)
	for node.next != nil && !node.next.Val.IsInf(0) && node.Val > node.next.Val {
		other := node.next
		m.swap(node, other) // node 换到 other 后面
		m.checkCross(node, other, true)
	}
	// 向左移动 (Val 变小)
	for node.prev != nil && !node.prev.Val.IsInf(0) && node.Val < node.prev.Val {
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

	// (3轴全部进入)
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
func (m *Manager) notifySubscribers(source *Entity, targetID aoi.EntityID, isEnter bool) {
	delta := -1
	if isEnter {
		delta = 1
	}

	for _, player := range source.Subscribers {
		m.refCountChange(player, targetID, delta)
	}
}

// refCountChange 玩家引用计数变更
func (m *Manager) refCountChange(p *aoi.Player, targetID aoi.EntityID, delta int) {
	oldVal := p.FinalView[targetID]
	newVal := oldVal + delta

	if newVal <= 0 {
		delete(p.FinalView, targetID)
	} else {
		p.FinalView[targetID] = newVal
	}

	// 触发回调 (0 -> 1 Enter, 1 -> 0 Leave)
	if m.eventCallback != nil {
		if oldVal == 0 && newVal > 0 {
			m.eventCallback.OnEnter(p.ID, targetID)
		} else if oldVal > 0 && newVal <= 0 {
			m.eventCallback.OnLeave(p.ID, targetID)
		}
	}
}
