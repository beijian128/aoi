package three_dim

import (
	"github.com/beijian128/aoi"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestAOI(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	// 初始化 NPC
	for i := aoi.EntityID(100); i < 200; i++ {
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
		world.AoiMgr.AddEntity(i, &aoi.Position{X: aoi.Float(obj.X), Y: aoi.Float(obj.Y), Z: aoi.Float(obj.Z)}, 5)
	}

	go gameLoop()

	// 设置静态文件服务，将 /static/ 映射到 ./static 目录
	fs := http.FileServer(http.Dir("./static"))

	// 将 /static/ 路径下的请求交给 FileServer 处理
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// 设置根路径 / 重定向到 /static/index.html
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/index.html", http.StatusFound)
	})

	http.HandleFunc("/ws", handleWebSocket)

	log.Println("3D Server started at http://localhost:8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

type SafeConn struct {
	Conn *websocket.Conn
	Mu   sync.Mutex
}

func (sc *SafeConn) WriteJSON(v interface{}) error {
	sc.Mu.Lock()
	defer sc.Mu.Unlock()
	return sc.Conn.WriteJSON(v)
}

type SimObject struct {
	ID aoi.EntityID `json:"id"`
	X  float64      `json:"x"`
	Y  float64      `json:"y"`
	Z  float64      `json:"z"`
	VX float64      `json:"-"`
	VY float64      `json:"-"`
	VZ float64      `json:"-"`
}

type PosUpdateMsg struct {
	Type string       `json:"type"`
	Data []*SimObject `json:"data"`
}

type ViewUpdateMsg struct {
	Type    string         `json:"type"`
	Visible []aoi.EntityID `json:"visible"`
}

type InitMsg struct {
	Type  string       `json:"type"`
	MyID  aoi.EntityID `json:"my_id"`
	Range float64      `json:"range"`
	NPCs  []*SimObject `json:"npcs"`
}

type World struct {
	AoiMgr  *Manager
	Objects map[aoi.EntityID]*SimObject
	Lock    sync.RWMutex
	Conns   map[*SafeConn]bool
}

var world = &World{
	AoiMgr:  NewManager(),
	Objects: make(map[aoi.EntityID]*SimObject),
	Conns:   make(map[*SafeConn]bool),
}

const MapSize = 500.0

type MyAOICallback struct {
	conn      *SafeConn
	mu        sync.Mutex
	visible   map[aoi.EntityID]bool
	sendTimer *time.Timer
}

func (cb *MyAOICallback) OnEnter(playerID aoi.PlayerID, targetID aoi.EntityID) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.visible[targetID] = true
	cb.triggerSend()
}

func (cb *MyAOICallback) OnLeave(playerID aoi.PlayerID, targetID aoi.EntityID) {
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
		list := make([]aoi.EntityID, 0, len(cb.visible))
		for id := range cb.visible {
			list = append(list, id)
		}
		cb.sendTimer = nil
		cb.mu.Unlock()

		_ = cb.conn.WriteJSON(ViewUpdateMsg{Type: "view_update", Visible: list})
	})
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func gameLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		world.Lock.Lock()

		for _, obj := range world.Objects {
			obj.X += obj.VX
			obj.Y += obj.VY
			obj.Z += obj.VZ

			if obj.X <= 0 || obj.X >= MapSize {
				obj.VX = -obj.VX
			}
			if obj.Y <= 0 || obj.Y >= MapSize {
				obj.VY = -obj.VY
				obj.VY = -obj.VY
			}
			if obj.Z <= 0 || obj.Z >= MapSize {
				obj.VZ = -obj.VZ
			}

			world.AoiMgr.MoveEntity(obj.ID, &aoi.Position{X: aoi.Float(obj.X), Y: aoi.Float(obj.Y), Z: aoi.Float(obj.Z)})
		}

		allPos := make([]*SimObject, 0, len(world.Objects))
		for _, obj := range world.Objects {
			allPos = append(allPos, obj)
		}
		msg := PosUpdateMsg{Type: "pos_update", Data: allPos}

		for safeConn := range world.Conns {
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

	safeConn := &SafeConn{Conn: rawConn}

	world.Lock.Lock()
	world.Conns[safeConn] = true
	world.Lock.Unlock()

	defer func() {
		world.Lock.Lock()
		delete(world.Conns, safeConn)
		world.AoiMgr.RemoveEntity(1)
		world.Lock.Unlock()
		rawConn.Close()
	}()

	playerID := aoi.PlayerID(1)
	playerEntityID := aoi.EntityID(1)
	viewRange := 80.0

	world.Lock.Lock()
	world.AoiMgr.AddEntity(playerEntityID, &aoi.Position{
		X: 250,
		Y: 250,
		Z: 250,
	}, aoi.Float(viewRange))
	world.AoiMgr.AddPlayer(playerID)
	world.AoiMgr.Subscribe(playerID, playerEntityID)

	cb := &MyAOICallback{conn: safeConn, visible: make(map[aoi.EntityID]bool)}
	world.AoiMgr.SetCallback(cb)

	initNPCs := make([]*SimObject, 0, len(world.Objects))
	for _, o := range world.Objects {
		initNPCs = append(initNPCs, o)
	}
	safeConn.WriteJSON(InitMsg{Type: "init", MyID: playerEntityID, Range: viewRange, NPCs: initNPCs})
	world.Lock.Unlock()

	// 接收循环
	for {
		var input struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			Z float64 `json:"z"`
		}
		err := safeConn.Conn.ReadJSON(&input)
		if err != nil {
			break
		}

		world.Lock.Lock()
		world.AoiMgr.MoveEntity(playerEntityID, &aoi.Position{X: aoi.Float(input.X), Y: aoi.Float(input.Y), Z: aoi.Float(input.Z)})
		world.Lock.Unlock()
	}
}
