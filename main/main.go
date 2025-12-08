package main

import (
	"fmt"
	"github.com/beijian128/aoi/xlist"
)

type GameLogic struct{}

func (l *GameLogic) OnPlayerEnter(pid, tid int64) {
	fmt.Printf("[AOI] Player %d SEES Entity %d\n", pid, tid)
}
func (l *GameLogic) OnPlayerLeave(pid, tid int64) {
	fmt.Printf("[AOI] Player %d LOST Entity %d\n", pid, tid)
}

func main() {
	fmt.Println("=== MOBA AOI Demo ===")

	m := xlist.NewManager()
	m.SetCallback(&GameLogic{})

	// 1. 创建玩家 P1
	p1ID := int64(100)
	m.AddPlayer(p1ID)

	// 2. 创建 英雄 (ID:1) 视野 10，位置 (0,0,0)
	fmt.Println("\n--- 1. Hero Spawn ---")
	m.AddEntity(1, 0, 0, 0, 10)
	// 绑定：英雄属于 P1
	m.Subscribe(p1ID, 1)

	// 3. 创建 敌人 (ID:2) 位置 (50,0,0)
	// 此时 P1 看不见敌人 (距离 50 > 10)
	fmt.Println("\n--- 2. Enemy Spawn far away ---")
	m.AddEntity(2, 50, 0, 0, 5)

	// 4. P1 插眼 (ID:3) 视野 10，位置 (45,0,0)
	// 眼距离敌人 5 (在视野内)。插眼瞬间 P1 应该看见敌人。
	fmt.Println("\n--- 3. Place Ward close to Enemy ---")
	m.AddEntity(3, 45, 0, 0, 10)
	// 订阅瞬间触发聚合
	m.Subscribe(p1ID, 3)
	// 预期输出: Player 100 SEES Entity 2

	// 5. 英雄走过来 (15,0,0) -> (45,0,0)
	// 英雄也看见了敌人。此时 P1 有两个单位看见敌人 (引用计数=2)。
	// 不应重复触发 SEES 事件。
	fmt.Println("\n--- 4. Hero moves close to Enemy ---")
	m.Move(1, 45, 0, 0)

	// 6. 眼排掉了 (取消订阅 + 移除)
	// 引用计数 2 -> 1。P1 依然看见敌人（因为英雄还在）。
	// 不应触发 LOST 事件。
	fmt.Println("\n--- 5. Ward Destroyed ---")
	m.Unsubscribe(p1ID, 3)
	m.RemoveEntity(3)

	// 7. 英雄离开
	// 引用计数 1 -> 0。
	// 预期输出: Player 100 LOST Entity 2
	fmt.Println("\n--- 6. Hero leaves ---")
	m.Move(1, 0, 0, 0)
}
