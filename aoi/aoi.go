package aoi

const delaySeconds = 0.5

type Position struct {
	X, Z float32 // xz轴 平行于地面
}

type Entity struct {
	id      uint32
	pos     *Position
	isActor bool
}

func NewEntity(id uint32, pos *Position, isActor bool) *Entity {
	return &Entity{
		id:      id,
		pos:     pos,
		isActor: isActor,
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
func (e *Entity) IsActor() bool {
	return e.isActor
}

// Grid 1个格子
type Grid struct {
	entities map[uint32]*Entity // 格子中的所有实体
	row, col int                // 格子所在的行和列
}
type Manager struct {
	grids      [][]*Grid // 整个地图划分的格子
	maxX, maxZ int
	gridSize   int
	entities   map[uint32]*Entity

	interests map[uint32]Set[uint32]        // key=实体ID ，value=感兴趣的实体
	delayDel  map[uint32]map[uint32]float32 // x 秒后才真正从视野中删除 （为了缓解频繁重复进出同一个AOI而触发大量事件回调的压力）

	onEnterAOI func(self, other uint32)
	onLeaveAOI func(self, other uint32)
}

func NewManager(gridSize, maxX, maxZ int) *Manager {
	m := &Manager{
		gridSize:  gridSize,
		maxX:      maxX,
		maxZ:      maxZ,
		grids:     make([][]*Grid, maxX/gridSize+1),
		interests: make(map[uint32]Set[uint32]),        // todo
		delayDel:  make(map[uint32]map[uint32]float32), // todo
		entities:  make(map[uint32]*Entity),
	}
	for i := range m.grids {
		m.grids[i] = make([]*Grid, maxZ/gridSize+1)
		for j := range m.grids[i] {
			m.grids[i][j] = &Grid{
				entities: make(map[uint32]*Entity),
				row:      i,
				col:      j,
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
func (m *Manager) getGridByPos(pos *Position) *Grid {
	if pos == nil {
		return nil
	}
	if pos.X < 0 || pos.X >= float32(m.maxX) || pos.Z < 0 || pos.Z >= float32(m.maxZ) {
		return nil
	}
	row := int(pos.X) / m.gridSize
	col := int(pos.Z) / m.gridSize
	return m.grids[row][col]
}

func (m *Manager) foreachSurroundEntities(pos *Position, f func(entity *Entity), exclude uint32) {

	row := int(pos.X) / m.gridSize
	col := int(pos.Z) / m.gridSize
	for i := row - 1; i <= row+1; i++ {
		for j := col - 1; j <= col+1; j++ {
			if i < 0 || i >= m.maxX/m.gridSize+1 || j < 0 || j >= m.maxZ/m.gridSize+1 {
				continue
			}
			for _, e := range m.grids[i][j].entities {
				if e.GetID() == exclude {
					continue
				}
				f(e)
			}
		}
	}
}

func (m *Manager) AddEntity(entity *Entity) {
	if entity == nil {
		return
	}
	grid := m.getGridByPos(entity.pos)
	if grid == nil {
		return
	}
	grid.entities[entity.GetID()] = entity
	m.entities[entity.GetID()] = entity
	m.foreachSurroundEntities(entity.pos, func(e *Entity) {
		m.onEnter(entity, e)
		if m.onEnterAOI != nil {
			m.onEnterAOI(entity.GetID(), e.GetID())
		}

	}, entity.GetID())
}

func (m *Manager) RemoveEntity(entity *Entity) {
	if entity == nil {
		return
	}
	grid := m.getGridByPos(entity.pos)
	if grid == nil {
		return
	}
	delete(grid.entities, entity.GetID())
	delete(m.entities, entity.GetID())
	m.foreachSurroundEntities(entity.pos, func(e *Entity) {
		if m.onLeaveAOI != nil {
			m.onLeaveAOI(entity.GetID(), e.GetID())
		}
		m.onLeave(entity, e)
	}, entity.GetID())
}

func (m *Manager) MoveEntity(id uint32, pos *Position) {
	entity := m.entities[id]
	if entity == nil {
		return
	}
	oldGrid := m.getGridByPos(entity.pos)
	newGrid := m.getGridByPos(pos)
	if oldGrid == nil || newGrid == nil || oldGrid == newGrid {
		return
	}
	delete(oldGrid.entities, id)

	oldAOI := NewSet[*Entity]()
	newAOI := NewSet[*Entity]()
	m.foreachSurroundEntities(entity.pos, func(e *Entity) {
		oldAOI.Add(e)
	}, entity.GetID())
	m.foreachSurroundEntities(pos, func(e *Entity) {
		newAOI.Add(e)
	}, entity.GetID())
	oldAOI.ForEach(func(other *Entity) {

		if !newAOI.Contains(other) {
			if m.onLeaveAOI != nil {
				m.onLeaveAOI(entity.GetID(), other.GetID())
			}
			m.onLeave(entity, other)

		}
	})
	newAOI.ForEach(func(other *Entity) {
		if !oldAOI.Contains(other) {
			m.onEnter(entity, other)
			if m.onEnterAOI != nil {
				m.onEnterAOI(entity.GetID(), other.GetID())
			}
		}
	})

	entity.SetPos(pos)
	newGrid.entities[id] = entity
}

func (m *Manager) OnTick(delta float32) {
	for self, others := range m.delayDel {
		for other := range others {
			others[other] -= delta
			if others[other] <= 0 {
				delete(others, other)
				delete(m.interests[self], other)
			}
		}
	}
}
func (m *Manager) IsInAOI(self, other uint32) bool {
	if m.interests[self] == nil {
		return false
	}
	return m.interests[self].Contains(other)
}
func (m *Manager) GetAOI(self uint32) Set[uint32] {
	return m.interests[self]
}

func (m *Manager) onEnter(e1, e2 *Entity) {
	addInterest := func(e1, e2 *Entity) {
		if !e1.IsActor() {
			return
		}
		if m.interests[e1.GetID()] == nil {
			m.interests[e1.GetID()] = NewSet[uint32]()
		}
		m.interests[e1.GetID()].Add(e2.GetID())
		delete(m.delayDel[e1.GetID()], e2.GetID())
	}
	addInterest(e1, e2)
	addInterest(e2, e1)
}
func (m *Manager) onLeave(e1, e2 *Entity) {
	delayDelFunc := func(e1, e2 *Entity) {
		if !e1.IsActor() {
			return
		}
		if m.delayDel[e1.GetID()] == nil {
			m.delayDel[e1.GetID()] = make(map[uint32]float32)
		}
		m.delayDel[e1.GetID()][e2.GetID()] = delaySeconds
	}
	delayDelFunc(e1, e2)
	delayDelFunc(e2, e1)
}
