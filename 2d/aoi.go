package two_dim

import (
	"github.com/beijian128/aoi"
)

type Entity struct {
	id  aoi.EntityID
	pos *aoi.Position

	subscribers map[aoi.PlayerID]*aoi.Player
}

func NewEntity(id aoi.EntityID, pos *aoi.Position) *Entity {
	return &Entity{
		id:          id,
		pos:         pos,
		subscribers: map[aoi.PlayerID]*aoi.Player{},
	}
}

func (e *Entity) GetID() aoi.EntityID {
	return e.id
}

func (e *Entity) GetPos() *aoi.Position {
	return e.pos
}

func (e *Entity) SetPos(pos *aoi.Position) {
	e.pos = pos
}

// Grid 1个格子
type Grid struct {
	entities map[aoi.EntityID]*Entity // 格子中的所有实体
}
type Manager struct {
	grids                  [][]*Grid
	minX, minZ, maxX, maxZ int
	gridSize               int
	rowNum, columnNum      int
	entities               map[aoi.EntityID]*Entity
	players                map[aoi.PlayerID]*aoi.Player

	cbk aoi.AOICallback
}

func (m *Manager) AddPlayer(id aoi.PlayerID) {
	m.players[id] = &aoi.Player{
		ID:        id,
		FinalView: make(map[aoi.EntityID]int),
	}
}
func (m *Manager) AddEntity(id aoi.EntityID, pos *aoi.Position, rangeVal aoi.Float) {
	if pos == nil {
		return
	}
	entity := NewEntity(id, pos)
	row, col := m.getGridIndexByPos(entity.GetPos())
	grid := m.grids[row][col] // 一定能找到，这里就不判空了
	grid.entities[entity.GetID()] = entity
	m.entities[entity.GetID()] = entity
	m.findSurroundEntities(entity).ForEach(func(other *Entity) bool {
		m.onEnter(entity, other)
		return false
	})
}

func (m *Manager) RemoveEntity(id aoi.EntityID) {
	entity := m.entities[id]
	if entity == nil {
		return
	}
	row, col := m.getGridIndexByPos(entity.GetPos())
	grid := m.grids[row][col]
	delete(grid.entities, id)
	delete(m.entities, id)
	m.findSurroundEntities(entity).ForEach(func(other *Entity) bool {
		m.onLeave(entity, other)
		return false
	})
}

func (m *Manager) MoveEntity(id aoi.EntityID, pos *aoi.Position) {
	if pos == nil {
		return
	}
	entity := m.entities[id]
	if entity == nil {
		return
	}

	oldRow, oldCol := m.getGridIndexByPos(entity.GetPos())
	newRow, newCol := m.getGridIndexByPos(pos)
	if oldRow == newRow && oldCol == newCol {
		entity.SetPos(pos)
		return
	}

	oldAOI := m.findSurroundEntities(entity)

	delete(m.grids[oldRow][oldCol].entities, entity.GetID())
	m.grids[newRow][newCol].entities[entity.GetID()] = entity

	entity.SetPos(pos)
	newAOI := m.findSurroundEntities(entity)

	leaveSet := oldAOI.Difference(newAOI)
	enterSet := newAOI.Difference(oldAOI)

	leaveSet.ForEach(func(other *Entity) bool {
		m.onLeave(entity, other)
		return false
	})
	enterSet.ForEach(func(other *Entity) bool {
		m.onEnter(entity, other)
		return false
	})
}

func (m *Manager) GetView(id aoi.PlayerID) aoi.Set[aoi.EntityID] {
	set := aoi.NewSet[aoi.EntityID]()
	player := m.players[id]
	if player == nil {
		return set
	}
	for eid, cnt := range player.FinalView {
		if cnt > 0 {
			set.Add(eid)
		}
	}
	return set
}

func (m *Manager) Subscribe(subscriberId aoi.PlayerID, targetId aoi.EntityID) {

	subscriber := m.players[subscriberId]
	target := m.entities[targetId]
	if subscriber == nil || target == nil {
		return
	}
	if _, ok := target.subscribers[subscriberId]; ok { // 已经订阅过
		return
	}
	target.subscribers[subscriberId] = subscriber
	m.findSurroundEntities(target).ForEach(func(other *Entity) bool {
		m.incrFinalView(subscriber, other)
		return false
	})
}

func (m *Manager) Unsubscribe(subscriberId aoi.PlayerID, targetId aoi.EntityID) {
	subscriber := m.players[subscriberId]
	target := m.entities[targetId]
	if subscriber == nil || target == nil {
		return
	}
	if _, ok := target.subscribers[subscriberId]; !ok { // 本来就没订阅
		return
	}
	delete(target.subscribers, subscriberId)
	m.findSurroundEntities(target).ForEach(func(other *Entity) bool {
		m.decrFinalView(subscriber, other)
		return false
	})
}

func (m *Manager) SetCallback(cb aoi.AOICallback) {
	m.cbk = cb
}

func NewManager(gridSize, minX, minZ, maxX, maxZ int) *Manager {
	m := &Manager{
		minX:      minX,
		minZ:      minZ,
		maxX:      maxX,
		maxZ:      maxZ,
		gridSize:  gridSize,
		entities:  make(map[aoi.EntityID]*Entity),
		players:   make(map[aoi.PlayerID]*aoi.Player),
		rowNum:    (maxX-minX)/gridSize + 1,
		columnNum: (maxZ-minZ)/gridSize + 1,
	}
	m.grids = make([][]*Grid, m.rowNum)
	for i := range m.grids {
		m.grids[i] = make([]*Grid, m.columnNum)
		for j := range m.grids[i] {
			m.grids[i][j] = &Grid{
				entities: make(map[aoi.EntityID]*Entity),
			}
		}
	}
	return m
}

func (m *Manager) getGridIndexByPos(pos *aoi.Position) (int, int) {
	row := (int(pos.X) - m.minX) / m.gridSize
	col := (int(pos.Z) - m.minZ) / m.gridSize
	if row < 0 {
		row = 0
	}
	if col < 0 {
		col = 0
	}
	if row >= m.rowNum {
		row = m.rowNum - 1
	}
	if col >= m.columnNum {
		col = m.columnNum - 1
	}
	return row, col
}

func (m *Manager) findSurroundEntities(e *Entity) aoi.Set[*Entity] {
	pos := e.GetPos()
	row, col := m.getGridIndexByPos(pos)
	set := aoi.NewSet[*Entity]()
	for i := row - 1; i <= row+1; i++ {
		for j := col - 1; j <= col+1; j++ {
			if i < 0 || i >= m.rowNum || j < 0 || j >= m.columnNum {
				continue
			}
			for _, v := range m.grids[i][j].entities {
				set.Add(v)
			}
		}
	}
	return set
}

func (m *Manager) CanSee(watcherId aoi.PlayerID, targetId aoi.EntityID) bool {
	watcher := m.players[watcherId]
	if watcher == nil {
		return false
	}
	return watcher.FinalView[targetId] > 0
}

func (m *Manager) incrFinalView(player *aoi.Player, e *Entity) {
	player.FinalView[e.GetID()]++
	if player.FinalView[e.GetID()] == 1 {
		if m.cbk != nil {
			m.cbk.OnEnter(player.ID, e.GetID())
		}
	}
}

func (m *Manager) decrFinalView(player *aoi.Player, e *Entity) {
	player.FinalView[e.GetID()]--
	if player.FinalView[e.GetID()] <= 0 {
		if m.cbk != nil {
			m.cbk.OnLeave(player.ID, e.GetID())
		}
		delete(player.FinalView, e.GetID())
	}
}

func (m *Manager) onEnter(e1, e2 *Entity) {
	for _, subscriber := range e1.subscribers {
		m.incrFinalView(subscriber, e2)
	}
	for _, subscriber := range e2.subscribers {
		m.incrFinalView(subscriber, e1)
	}
}

func (m *Manager) onLeave(e1, e2 *Entity) {
	for _, subscriber := range e1.subscribers {
		m.decrFinalView(subscriber, e2)
	}
	for _, subscriber := range e2.subscribers {
		m.decrFinalView(subscriber, e1)
	}
}
