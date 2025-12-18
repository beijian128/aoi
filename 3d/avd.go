package three_dim

import (
	"github.com/beijian128/aoi"
)

// === DTO (Data Transfer Objects) 用于 JSON 序列化 ===

type DebugSnapshot struct {
	Entities  []DebugEntity   `json:"ents"`
	Relations []DebugRelation `json:"rels"`
}

type DebugEntity struct {
	ID    int64      `json:"id"`
	Type  string     `json:"type"` // "player" 或 "npc"
	Pos   [3]float64 `json:"pos"`
	Range [3]float64 `json:"range"`
}

type DebugRelation struct {
	WatcherID int64 `json:"wid"`
	TargetID  int64 `json:"tid"`
}

// MakeSnapshot 生成当前时刻的深拷贝快照
// 必须在游戏主逻辑线程中调用 (非并发安全，但读取安全)
func (m *Manager) MakeSnapshot() *DebugSnapshot {
	snap := &DebugSnapshot{
		Entities:  make([]DebugEntity, 0, len(m.entities)),
		Relations: make([]DebugRelation, 0),
	}

	for id, e := range m.entities {
		// 1. 转换实体数据
		dEnt := DebugEntity{
			ID: int64(id),
			Pos: [3]float64{
				float64(e.Pos[0]), float64(e.Pos[1]), float64(e.Pos[2]),
			},
			// 你现在的代码 Range 是单个 Float (立方体)，这里转为数组传给前端以便兼容长方体逻辑
			Range: [3]float64{
				float64(e.Range), float64(e.Range), float64(e.Range),
			},
			Type: "npc",
		}

		// 2. 检查是否是 Player (逻辑层)
		if p, isPlayer := m.players[aoi.PlayerID(id)]; isPlayer {
			dEnt.Type = "player"

			// 3. 只有 Player 才有 FinalView (视野关系)
			// 注意：这里假设 PlayerID == EntityID (通常绑定的情况)
			for targetID := range p.FinalView {
				snap.Relations = append(snap.Relations, DebugRelation{
					WatcherID: int64(id), // 认为是该 Entity 看见的
					TargetID:  int64(targetID),
				})
			}
		}

		snap.Entities = append(snap.Entities, dEnt)
	}

	return snap
}
