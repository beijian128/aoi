package two_dim

import (
	"fmt"
	"github.com/beijian128/aoi"
	"github.com/gorilla/websocket"
	"golang.org/x/exp/rand"
	"log"
	"math"
	"net/http"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestAOI(t *testing.T) {
	mgr = NewManager(GridSize, 0, 0, MapSize, MapSize)

	for i := 1; i <= 20; i++ {
		mgr.AddEntity(aoi.EntityID(i), getRandPos(), 0)
	}
	pid := aoi.PlayerID(100)
	wardId := aoi.EntityID(200)
	mgr.AddPlayer(pid)
	mgr.AddEntity(aoi.EntityID(pid), getRandPos(), 0)
	mgr.Subscribe(pid, aoi.EntityID(pid))
	mgr.AddEntity(wardId, getRandPos(), 0)
	mgr.Subscribe(pid, wardId)
	go simulationLoop()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/ws", handleWS)
	fmt.Printf("服务启动: http://localhost%s\n", Port)
	fmt.Println("蓝色是主角，绿色是眼(被订阅)，红色是NPC")
	time.AfterFunc(time.Millisecond*100, func() {
		cmd := exec.Command("cmd", "/c", "start ", fmt.Sprintf(" http://localhost%s", Port))
		cmd.Start()
	})
	log.Fatal(http.ListenAndServe(Port, nil))
}

const (
	MapSize  = 600
	GridSize = 50
	Port     = ":8080"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Snapshot struct {
	Entities []EntityDTO `json:"entities"`
	Lines    []LineDTO   `json:"lines"` // 视野连线
}

type EntityDTO struct {
	ID    aoi.EntityID `json:"id"`
	X     aoi.Float    `json:"x"`
	Z     aoi.Float    `json:"z"`
	Type  string       `json:"type"` // "player", "npc", "ward"
	Color string       `json:"color"`
}

type LineDTO struct {
	FromX aoi.Float `json:"from_x"`
	FromZ aoi.Float `json:"from_z"`
	ToX   aoi.Float `json:"to_x"`
	ToZ   aoi.Float `json:"to_z"`
	Color string    `json:"color"`
}

var (
	mgr *Manager
	mu  sync.Mutex
)

type NPCState struct {
	vx, vz     aoi.Float
	changeTime time.Time
}

var npcStates = make(map[aoi.EntityID]*NPCState)

func getRandPos() *aoi.Position {
	return &aoi.Position{X: aoi.Float(rand.Intn(MapSize)), Y: 0, Z: aoi.Float(rand.Intn(MapSize))}
}
func simulationLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	const speed = 3.0
	for range ticker.C {
		mu.Lock()
		for id, e := range mgr.entities {
			if id < 100 {
				state, ok := npcStates[id]
				if !ok || time.Now().After(state.changeTime) {

					angle := rand.Float64() * 2 * math.Pi
					state = &NPCState{
						vx: aoi.Float(math.Cos(angle)) * speed,
						vz: aoi.Float(math.Sin(angle)) * speed,
						// 随机走 1 到 4 秒后再换方向
						changeTime: time.Now().Add(time.Duration(rand.Intn(3000)+1000) * time.Millisecond),
					}
					npcStates[id] = state
				}

				pos := e.GetPos()
				newX := pos.X + state.vx
				newZ := pos.Z + state.vz

				if newX <= 0 || newX >= MapSize {
					state.vx = -state.vx    // X轴速度反转
					newX = pos.X + state.vx // 重新计算位置
				}
				if newZ <= 0 || newZ >= MapSize {
					state.vz = -state.vz // Z轴速度反转
					newZ = pos.Z + state.vz
				}

				mgr.MoveEntity(id, &aoi.Position{X: newX, Z: newZ})
			}
		}
		mu.Unlock()
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	// 监听前端控制消息（控制主角移动）
	go func() {
		for {
			var msg struct {
				X aoi.Float `json:"x"`
				Z aoi.Float `json:"z"`
			}
			if err := ws.ReadJSON(&msg); err != nil {
				break
			}
			mu.Lock()
			// 移动主角 ID 100
			if _, ok := mgr.entities[100]; ok {
				mgr.MoveEntity(100, &aoi.Position{X: msg.X, Z: msg.Z})
			}
			mu.Unlock()
		}
	}()

	// 发送状态给前端
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		snapshot := buildSnapshot()
		if err := ws.WriteJSON(snapshot); err != nil {
			break
		}
	}
}

func buildSnapshot() Snapshot {
	mu.Lock()
	defer mu.Unlock()

	var snap Snapshot
	snap.Entities = make([]EntityDTO, 0, len(mgr.entities))
	snap.Lines = make([]LineDTO, 0)

	for id, e := range mgr.entities {
		pos := e.GetPos()
		eType := "npc"
		color := "#ff4d4f" // 红

		if id == 100 {
			eType = "player"
			color = "#1890ff" // 蓝
		} else if id == 200 {
			eType = "ward"
			color = "#52c41a" // 绿
		}

		snap.Entities = append(snap.Entities, EntityDTO{
			ID:    id,
			X:     pos.X,
			Z:     pos.Z,
			Type:  eType,
			Color: color,
		})

		// 获取 AOI 列表生成连线
		// 为了不让画面太乱，只画 主角(100) 和 眼(200) 看到的物体
		if id == 100 || id == 200 {
			rawAoiSet := mgr.findSurroundEntities(e)
			aoiSet := mgr.GetView(aoi.PlayerID(id))
			aoiSet.ForEach(func(targetID aoi.EntityID) bool {
				target := mgr.entities[targetID]
				if target != nil {
					lineColor := "rgba(24, 144, 255, 0.3)" // 默认蓝色连线
					if id == 200 {
						lineColor = "rgba(82, 196, 26, 0.3)" // 眼的视野是绿色连线
					}

					// 如果是主角(100)看到了目标，但这个目标实际上是在眼(200)的周围
					// 这证明了 subscribe 功能生效
					if id == 100 {
						if !rawAoiSet.Contains(target) {
							lineColor = "rgba(255, 255, 0, 0.5)" // 黄色表示共享视野
						}
					}

					snap.Lines = append(snap.Lines, LineDTO{
						FromX: pos.X, FromZ: pos.Z,
						ToX: target.GetPos().X, ToZ: target.GetPos().Z,
						Color: lineColor,
					})
				}
				return false
			})
		}
	}
	return snap
}
