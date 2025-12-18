package three_dim

import (
	"encoding/json"
	"testing"

	"log"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/beijian128/aoi"

	"github.com/gorilla/websocket"
)

func TestAVD(t *testing.T) {
	// 创建几个预设房间
	CreateRoom(101, 1.0) // 正常速度
	CreateRoom(102, 2.0) // 快
	CreateRoom(103, 0.5) // 慢

	http.HandleFunc("/ws", wsHandler)
	http.Handle("/", http.FileServer(http.Dir("./static")))

	log.Println("Server running at http://localhost:8085/avd.html")
	log.Fatal(http.ListenAndServe(":8085", nil))
}

// === 快照存储 ===
type SnapshotStore struct {
	sync.RWMutex
	data *DebugSnapshot
}

func (s *SnapshotStore) Set(snap *DebugSnapshot) {
	s.Lock()
	defer s.Unlock()
	s.data = snap
}

func (s *SnapshotStore) Get() []byte {
	s.RLock()
	defer s.RUnlock()
	if s.data == nil {
		return nil
	}
	bytes, _ := json.Marshal(s.data)
	return bytes
}

// === 房间定义 ===
type GameRoom struct {
	ID          uint64
	AOIMgr      *Manager
	Store       *SnapshotStore
	SpeedFactor float64
}

var (
	RoomManager = make(map[uint64]*GameRoom)
	RoomLock    sync.RWMutex
)

func GetRoom(roomId uint64) *GameRoom {
	RoomLock.RLock()
	defer RoomLock.RUnlock()
	return RoomManager[roomId]
}

// 创建并初始化房间
func CreateRoom(roomId uint64, speed float64) *GameRoom {
	RoomLock.Lock()
	defer RoomLock.Unlock()

	if _, exists := RoomManager[roomId]; exists {
		return RoomManager[roomId]
	}

	aoiMgr := NewManager() // 这里的 NewManager 是你代码中的无参版本
	room := &GameRoom{
		ID:          roomId,
		AOIMgr:      aoiMgr,
		Store:       &SnapshotStore{},
		SpeedFactor: speed,
	}

	// === 初始化场景 ===

	// Player 1: 视野半径 20
	aoiMgr.AddPlayer(1)
	aoiMgr.AddEntity(1, &aoi.Position{X: 0, Y: 0, Z: 0}, 20.0)
	aoiMgr.Subscribe(1, 1) // 绑定逻辑与物理

	// Player 6: 第二个玩家，视野半径 15
	aoiMgr.AddPlayer(6)
	aoiMgr.AddEntity(6, &aoi.Position{X: -20, Y: 10, Z: -20}, 15.0)
	aoiMgr.Subscribe(6, 6) // 绑定逻辑与物理

	// NPCs: Range设为0 (只被看，不主动看)
	aoiMgr.AddEntity(2, &aoi.Position{X: 10, Y: 0, Z: 0}, 0)
	aoiMgr.AddEntity(3, &aoi.Position{X: -30, Y: 0, Z: -30}, 0)
	aoiMgr.AddEntity(4, &aoi.Position{X: 30, Y: 0, Z: 30}, 0)
	aoiMgr.AddEntity(5, &aoi.Position{X: 0, Y: 20, Z: 0}, 0)

	RoomManager[roomId] = room
	go room.runLoop() // 启动独立循环

	log.Printf("Room %d created (Speed: %.1f)", roomId, speed)
	return room
}

// 游戏逻辑循环
func (room *GameRoom) runLoop() {
	ticker := time.NewTicker(50 * time.Millisecond) // 20 TPS
	defer ticker.Stop()

	t := 0.0
	m := room.AOIMgr
	speed := room.SpeedFactor

	for range ticker.C {
		t += 0.05 * speed

		// Player 1: 圆周运动
		p1x := 35 * math.Cos(t)
		p1z := 35 * math.Sin(t)
		m.MoveEntity(1, &aoi.Position{X: aoi.Float(p1x), Y: 0, Z: aoi.Float(p1z)})

		// Player 6: 椭圆运动
		p6x := 25 * math.Cos(-t*0.8)
		p6z := 40 * math.Sin(-t*0.8)
		m.MoveEntity(6, &aoi.Position{X: aoi.Float(p6x), Y: 10, Z: aoi.Float(p6z)})

		// NPC 2: 快速穿梭
		n2x := 40 * math.Cos(t*1.5)
		m.MoveEntity(2, &aoi.Position{X: aoi.Float(n2x), Y: 5, Z: 0})

		// 生成快照
		room.Store.Set(m.MakeSnapshot())
	}
}

//// === WebSocket ===
//var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("roomId")
	if idStr == "" {
		http.Error(w, "Missing roomId", 400)
		return
	}
	roomId, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid roomId", 400)
		return
	}

	room := GetRoom(roomId)
	if room == nil {
		http.Error(w, "Room not found", 404)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 前端推送频率 (~30 FPS)
	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		data := room.Store.Get()
		if data != nil {
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				break
			}
		}
	}
}
