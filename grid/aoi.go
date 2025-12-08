package grid

import (
	"github.com/beijian128/aoi/common"
)

type Position struct {
	X, Z float32 // xz轴 平行于地面
}

type Entity struct {
	id  uint32
	pos *Position

	beSubscribed common.Set[uint32]
	interests    map[uint32]int
}

func NewEntity(id uint32, pos *Position) *Entity {
	return &Entity{
		id:           id,
		pos:          pos,
		beSubscribed: common.NewSet[uint32](),
		interests:    map[uint32]int{},
	}
}

func (e *Entity) GetID() uint32 {
	return e.id
}

func (e *Entity) GetPos() *Position {
	return e.pos
}

func (e *Entity) SetPos(pos *Position) {
	e.pos = pos
}

// Grid 1个格子
type Grid struct {
	entities map[uint32]*Entity // 格子中的所有实体
}
type Manager struct {
	grids                  [][]*Grid
	minX, minZ, maxX, maxZ int
	gridSize               int
	rowNum, columnNum      int
	entities               map[uint32]*Entity

	onEnterAOI func(self, other uint32)
	onLeaveAOI func(self, other uint32)
}

func NewManager(gridSize, minX, minZ, maxX, maxZ int) *Manager {
	m := &Manager{
		minX:      minX,
		minZ:      minZ,
		maxX:      maxX,
		maxZ:      maxZ,
		gridSize:  gridSize,
		entities:  make(map[uint32]*Entity),
		rowNum:    (maxX-minX)/gridSize + 1,
		columnNum: (maxZ-minZ)/gridSize + 1,
	}
	m.grids = append(m.grids, make([]*Grid, m.rowNum))
	for i := range m.grids {
		m.grids[i] = make([]*Grid, m.columnNum)
		for j := range m.grids[i] {
			m.grids[i][j] = &Grid{
				entities: make(map[uint32]*Entity),
			}
		}
	}
	return m
}

func (m *Manager) RegisterEnterAOIHandler(onEnterAOI func(self, other uint32)) {
	m.onEnterAOI = onEnterAOI
}

func (m *Manager) RegisterLeaveAOIHandler(onLeaveAOI func(self, other uint32)) {
	m.onLeaveAOI = onLeaveAOI
}

func (m *Manager) getGridIndexByPos(pos *Position) (int, int) {
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

func (m *Manager) findSurroundEntities(e *Entity) common.Set[*Entity] {
	pos := e.GetPos()
	row, col := m.getGridIndexByPos(pos)
	set := common.NewSet[*Entity]()
	for i := row - 1; i <= row+1; i++ {
		for j := col - 1; j <= col+1; j++ {
			if i < 0 || i >= m.rowNum || j < 0 || j >= m.columnNum {
				continue
			}
			for _, v := range m.grids[i][j].entities {
				if v.GetID() == e.GetID() {
					continue
				}
				set.Add(e)
			}
		}
	}
	return set
}

func (m *Manager) AddEntity(entity *Entity) {
	if entity == nil {
		return
	}
	row, col := m.getGridIndexByPos(entity.GetPos())
	grid := m.grids[row][col] // 一定能找到，这里就不判空了
	grid.entities[entity.GetID()] = entity
	m.entities[entity.GetID()] = entity
	m.findSurroundEntities(entity).ForEach(func(other *Entity) bool {
		m.onEnter(entity, other)
		return false
	})
}

func (m *Manager) RemoveEntity(id uint32) {
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

func (m *Manager) MoveEntity(id uint32, pos *Position) {
	entity := m.entities[id]
	if entity == nil {
		return
	}
	oldAOI := m.findSurroundEntities(entity)
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

// CanSee self是否能看到other
func (m *Manager) CanSee(selfId, otherId uint32) bool {
	self := m.entities[selfId]
	if self == nil {
		return false
	}
	return self.interests[otherId] > 0
}

func (m *Manager) GetAOI(id uint32) common.Set[uint32] {
	set := common.NewSet[uint32]()
	e := m.entities[id]
	if e == nil {
		return set
	}
	for eid, cnt := range e.interests {
		if cnt > 0 {
			set.Add(eid)
		}
	}
	return set
}

func (m *Manager) addInterest(e1, e2 *Entity) {
	e1.interests[e2.GetID()]++
	if e1.interests[e2.GetID()] == 1 {
		if m.onEnterAOI != nil {
			m.onEnterAOI(e1.GetID(), e2.GetID())
		}
	}
}

func (m *Manager) removeInterest(e1, e2 *Entity) {
	e1.interests[e2.GetID()]--
	if e1.interests[e2.GetID()] == 0 {
		if m.onLeaveAOI != nil {
			m.onLeaveAOI(e1.GetID(), e2.GetID())
		}
	}
}

func (m *Manager) onEnter(e1, e2 *Entity) {
	m.addInterest(e1, e2)
	m.addInterest(e2, e1)
	e1.beSubscribed.ForEach(func(id uint32) bool {
		e := m.entities[id]
		if e != nil {
			m.addInterest(e, e2)
		}
		return false
	})
	e2.beSubscribed.ForEach(func(id uint32) bool {
		e := m.entities[id]
		if e != nil {
			m.addInterest(e, e1)
		}
		return false
	})
}

func (m *Manager) onLeave(e1, e2 *Entity) {
	m.removeInterest(e1, e2)
	m.removeInterest(e2, e1)
}

// Subscribe e1 订阅 e2 . 进入e2视野的实体将同时进入e1的视野
// 一个可能的应用场景：MOBA游戏，玩家控制的角色插眼 ，玩家的视野变为角色的视野加上眼的视野，让角色实体订阅“眼”实体即可
// 订阅不具有传递性，仅将被订阅目标的原始视野合并到订阅者上
func (m *Manager) Subscribe(eid1, eid2 uint32) {
	if eid1 == eid2 {
		return
	}
	e1 := m.entities[eid1]
	e2 := m.entities[eid2]
	if e1 == nil || e2 == nil {
		return
	}
	if e2.beSubscribed.Contains(e1.GetID()) {
		return
	}
	e2.beSubscribed.Add(e1.GetID())
	m.findSurroundEntities(e2).ForEach(func(other *Entity) bool { // 原始视野
		m.addInterest(e1, other)
		return false
	})

}

func (m *Manager) Unsubscribe(eid1, eid2 uint32) {
	if eid1 == eid2 {
		return
	}
	e1 := m.entities[eid1]
	e2 := m.entities[eid2]
	if e1 == nil || e2 == nil {
		return
	}
	if !e2.beSubscribed.Contains(e1.GetID()) {
		return
	}
	e2.beSubscribed.Remove(e1.GetID())
	m.findSurroundEntities(e2).ForEach(func(other *Entity) bool {
		m.removeInterest(e1, other)
		return false
	})
}
