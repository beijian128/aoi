package aoi

import (
	"math"
)

type (
	EntityID int64
	PlayerID int64
	Float    float64
)

func (f *Float) IsInf(sign int) bool {
	return math.IsInf(float64(*f), sign)
}

func FloatInf(sign int) Float {
	return Float(math.Inf(sign))
}

// Position 通用位置结构
type Position struct {
	X, Y, Z Float
}

// Player 玩家 (逻辑层)
// 它是视野的订阅者，本身没有坐标，汇总其下属 Entity 的视野
type Player struct {
	ID PlayerID
	// FinalView: 聚合后的视野
	// Key: TargetID, Value: 引用计数 (有多少个我的单位看见了这个目标)
	FinalView map[EntityID]int
}

// AOICallback 回调接口：处理视野进出事件
type AOICallback interface {
	// OnEnter watcher 看到了 target
	OnEnter(watcherID PlayerID, targetID EntityID)
	// OnLeave watcher 看不见 target 了
	OnLeave(watcherID PlayerID, targetID EntityID)
}

type AOIManager interface {
	AddPlayer(id PlayerID)
	AddEntity(id EntityID, pos *Position, rangeVal Float)
	RemoveEntity(id EntityID)
	MoveEntity(id EntityID, pos *Position)
	// GetView 获取视野内所有目标 ID
	GetView(id PlayerID) Set[EntityID]
	// CanSee watcherId 是否能看见 targetId
	CanSee(watcherId PlayerID, targetId EntityID) bool
	// Subscribe 视野订阅 (id1 共享 id2 的视野) - 可选特性
	Subscribe(subscriber PlayerID, target EntityID)
	// Unsubscribe 取消订阅
	Unsubscribe(subscriber PlayerID, target EntityID)
	// SetCallback 设置上层业务回调
	SetCallback(cb AOICallback)
}
