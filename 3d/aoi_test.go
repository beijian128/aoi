package three_dim

import (
	"log"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestAOI(t *testing.T) {
	main()
}

// ----------------------
// 1. 线程安全的 WebSocket 封装 (新增)
// ----------------------
type SafeConn struct {
	Conn *websocket.Conn
	Mu   sync.Mutex
}

// WriteJSON 线程安全的写入方法
func (sc *SafeConn) WriteJSON(v interface{}) error {
	sc.Mu.Lock()
	defer sc.Mu.Unlock()
	return sc.Conn.WriteJSON(v)
}

// ----------------------
// 数据结构
// ----------------------

type SimObject struct {
	ID int64   `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	Z  float64 `json:"z"`
	VX float64 `json:"-"`
	VY float64 `json:"-"`
	VZ float64 `json:"-"`
}

type PosUpdateMsg struct {
	Type string       `json:"type"`
	Data []*SimObject `json:"data"`
}

type ViewUpdateMsg struct {
	Type    string  `json:"type"`
	Visible []int64 `json:"visible"`
}

type InitMsg struct {
	Type  string       `json:"type"`
	MyID  int64        `json:"my_id"`
	Range float64      `json:"range"`
	NPCs  []*SimObject `json:"npcs"`
}

// ----------------------
// 全局状态
// ----------------------

type World struct {
	AoiMgr  *Manager
	Objects map[int64]*SimObject
	Lock    sync.RWMutex
	// 修改：存储 SafeConn 指针
	Conns map[*SafeConn]bool
}

var world = &World{
	AoiMgr:  NewManager(),
	Objects: make(map[int64]*SimObject),
	Conns:   make(map[*SafeConn]bool),
}

const MapSize = 500.0

// ----------------------
// AOI 回调
// ----------------------

type MyAOICallback struct {
	// 修改：使用 SafeConn
	conn      *SafeConn
	mu        sync.Mutex
	visible   map[int64]bool
	sendTimer *time.Timer
}

func (cb *MyAOICallback) OnPlayerEnter(playerID, targetID int64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.visible[targetID] = true
	cb.triggerSend()
}

func (cb *MyAOICallback) OnPlayerLeave(playerID, targetID int64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.visible, targetID)
	cb.triggerSend()
}

func (cb *MyAOICallback) triggerSend() {
	if cb.sendTimer != nil {
		return
	}
	// 防抖发送
	cb.sendTimer = time.AfterFunc(20*time.Millisecond, func() {
		cb.mu.Lock()
		// 复制一份数据，尽快释放锁
		list := make([]int64, 0, len(cb.visible))
		for id := range cb.visible {
			list = append(list, id)
		}
		cb.sendTimer = nil
		cb.mu.Unlock()

		// 发送数据 (SafeConn 内部有锁，这里不需要外部锁)
		_ = cb.conn.WriteJSON(ViewUpdateMsg{Type: "view_update", Visible: list})
	})
}

// ----------------------
// 主逻辑
// ----------------------

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func main() {
	rand.Seed(time.Now().UnixNano())

	// 初始化 NPC
	for i := int64(100); i < 200; i++ {
		obj := &SimObject{
			ID: i,
			X:  rand.Float64() * MapSize,
			Y:  rand.Float64() * MapSize,
			Z:  rand.Float64() * MapSize,
			VX: (rand.Float64() - 0.5) * 2,
			VY: (rand.Float64() - 0.5) * 2,
			VZ: (rand.Float64() - 0.5) * 2,
		}
		world.Objects[i] = obj
		world.AoiMgr.AddEntity(i, obj.X, obj.Y, obj.Z, 5)
	}

	go gameLoop()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	http.HandleFunc("/ws", handleWebSocket)

	log.Println("3D Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func gameLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		world.Lock.Lock()

		// 1. 移动 NPC
		for _, obj := range world.Objects {
			obj.X += obj.VX
			obj.Y += obj.VY
			obj.Z += obj.VZ

			if obj.X <= 0 || obj.X >= MapSize {
				obj.VX = -obj.VX
			}
			if obj.Y <= 0 || obj.Y >= MapSize {
				obj.VY = -obj.VY
			}
			if obj.Z <= 0 || obj.Z >= MapSize {
				obj.VZ = -obj.VZ
			}

			world.AoiMgr.Move(obj.ID, obj.X, obj.Y, obj.Z)
		}

		// 2. 准备广播数据
		allPos := make([]*SimObject, 0, len(world.Objects))
		for _, obj := range world.Objects {
			allPos = append(allPos, obj)
		}
		msg := PosUpdateMsg{Type: "pos_update", Data: allPos}

		// 3. 广播 (使用 SafeConn 避免并发 Panic)
		for safeConn := range world.Conns {
			// 注意：这里忽略错误，实际生产中如果报错需要移除连接
			go func(sc *SafeConn) {
				_ = sc.WriteJSON(msg)
			}(safeConn)
		}

		world.Lock.Unlock()
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	rawConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// 包装连接
	safeConn := &SafeConn{Conn: rawConn}

	world.Lock.Lock()
	world.Conns[safeConn] = true
	world.Lock.Unlock()

	defer func() {
		world.Lock.Lock()
		delete(world.Conns, safeConn)
		world.AoiMgr.RemoveEntity(1) // 简单处理：假设 ID 为 1
		world.Lock.Unlock()
		rawConn.Close()
	}()

	playerID := int64(1) // 演示用固定ID，多人测试需改为动态生成
	viewRange := 80.0

	world.Lock.Lock()
	world.AoiMgr.AddEntity(playerID, 250, 250, 250, viewRange)
	world.AoiMgr.AddPlayer(playerID)
	world.AoiMgr.Subscribe(playerID, playerID)

	// 回调中使用 safeConn
	cb := &MyAOICallback{conn: safeConn, visible: make(map[int64]bool)}
	world.AoiMgr.SetCallback(cb)

	// 发送初始包
	initNPCs := make([]*SimObject, 0, len(world.Objects))
	for _, o := range world.Objects {
		initNPCs = append(initNPCs, o)
	}
	// 使用 safeConn 发送
	safeConn.WriteJSON(InitMsg{Type: "init", MyID: playerID, Range: viewRange, NPCs: initNPCs})
	world.Lock.Unlock()

	// 接收循环
	for {
		var input struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			Z float64 `json:"z"`
		}
		// ReadJSON 是读操作，和 WriteJSON 互斥锁不冲突，可以直接用 rawConn 或 safeConn.Conn
		err := safeConn.Conn.ReadJSON(&input)
		if err != nil {
			break
		}

		world.Lock.Lock()
		world.AoiMgr.Move(playerID, input.X, input.Y, input.Z)
		world.Lock.Unlock()
	}
}
