package main

import (
	"fmt"
	"github.com/beijian128/aoi/grid"
	"github.com/gorilla/websocket"
	"log"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// 配置常量
const (
	MapSize  = 600
	GridSize = 50
	Port     = ":8080"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// 传输给前端的数据结构
type Snapshot struct {
	Entities []EntityDTO `json:"entities"`
	Lines    []LineDTO   `json:"lines"` // 视野连线
}

type EntityDTO struct {
	ID    uint32  `json:"id"`
	X     float32 `json:"x"`
	Z     float32 `json:"z"`
	Type  string  `json:"type"` // "player", "npc", "ward"
	Color string  `json:"color"`
}

type LineDTO struct {
	FromX float32 `json:"from_x"`
	FromZ float32 `json:"from_z"`
	ToX   float32 `json:"to_x"`
	ToZ   float32 `json:"to_z"`
	Color string  `json:"color"`
}

// 全局状态
var (
	mgr      *grid.Manager
	entities map[uint32]*grid.Entity
	mu       sync.Mutex
)

func main() {
	// 1. 初始化 AOI 管理器
	// 地图 0,0 到 600,600，格子大小 50
	mgr = grid.NewManager(GridSize, 0, 0, MapSize, MapSize)
	mgr.RegisterEnterAOIHandler(func(self, other uint32) {
		fmt.Printf("AOI: %d 进入 %d\n", self, other)
	})
	mgr.RegisterLeaveAOIHandler(func(self, other uint32) {
		fmt.Printf("AOI: %d 离开 %d\n", self, other)
	})
	entities = make(map[uint32]*grid.Entity)

	// 2. 添加 NPC (红色，随机移动)
	for i := 1; i <= 20; i++ {
		addEntity(uint32(i), float32(rand.Intn(MapSize)), float32(rand.Intn(MapSize)), "npc")
	}

	// 3. 添加主角 (蓝色 ID: 100)
	player := addEntity(100, 300, 300, "player")

	// 4. 添加一个静止的“眼/守卫” (绿色 ID: 200)
	ward := addEntity(200, 111.9, 111, "ward")

	// 5. 让主角订阅眼的视野 (Subscribe 演示)
	// 这样，在前端你即使离眼很远，也能看到眼周围的连线连接到主角身上（逻辑上）
	mgr.Subscribe(player.GetID(), ward.GetID())

	// 启动模拟循环
	go simulationLoop()

	// 启动 Web 服务器
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/ws", handleWS)

	fmt.Printf("服务启动: http://localhost%s\n", Port)
	fmt.Println("蓝色是主角，绿色是眼(被订阅)，红色是NPC")
	log.Fatal(http.ListenAndServe(Port, nil))
}

func addEntity(id uint32, x, z float32, t string) *grid.Entity {
	pos := &grid.Position{X: x, Z: z}
	e := grid.NewEntity(id, pos)

	// Hack: 我们可以把类型存在 Entity 结构体里，但这里为了不改你的代码，
	// 我们只在转换 JSON 时根据 ID 判断

	mgr.AddEntity(e)
	entities[id] = e
	return e
}

// 新增：用来保存 NPC 移动状态的结构
type NPCState struct {
	vx, vz     float32   // 当前 X, Z 轴的速度
	changeTime time.Time // 下次改变方向的时间
}

// 全局增加一个状态表
var npcStates = make(map[uint32]*NPCState)

// 模拟循环：让 NPC 移动
func simulationLoop() {
	ticker := time.NewTicker(50 * time.Millisecond) // 20 FPS
	// 定义移动速度 (每帧移动多少像素)
	const speed = 3.0
	for range ticker.C {
		mu.Lock()
		for id, e := range entities {
			// 只有 NPC (ID < 100) 会自动移动
			if id < 100 {
				// 1. 获取或初始化该 NPC 的状态
				state, ok := npcStates[id]
				if !ok || time.Now().After(state.changeTime) {
					// 需要初始化，或者到了改变方向的时间
					// 随机生成一个方向向量
					angle := rand.Float64() * 2 * math.Pi
					state = &NPCState{
						vx: float32(math.Cos(angle)) * speed,
						vz: float32(math.Sin(angle)) * speed,
						// 随机走 1 到 4 秒后再换方向
						changeTime: time.Now().Add(time.Duration(rand.Intn(3000)+1000) * time.Millisecond),
					}
					npcStates[id] = state
				}

				// 2. 计算新位置
				pos := e.GetPos()
				newX := pos.X + state.vx
				newZ := pos.Z + state.vz

				// 3. 边界反弹处理 (碰到墙壁反向反弹)
				if newX <= 0 || newX >= MapSize {
					state.vx = -state.vx    // X轴速度反转
					newX = pos.X + state.vx // 重新计算位置
				}
				if newZ <= 0 || newZ >= MapSize {
					state.vz = -state.vz // Z轴速度反转
					newZ = pos.Z + state.vz
				}

				// 4. 更新 Grid 管理器
				mgr.MoveEntity(id, &grid.Position{X: newX, Z: newZ})
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
				X float32 `json:"x"`
				Z float32 `json:"z"`
			}
			if err := ws.ReadJSON(&msg); err != nil {
				break
			}
			mu.Lock()
			// 移动主角 ID 100
			if _, ok := entities[100]; ok {
				mgr.MoveEntity(100, &grid.Position{X: msg.X, Z: msg.Z})
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
	snap.Entities = make([]EntityDTO, 0, len(entities))
	snap.Lines = make([]LineDTO, 0)

	for id, e := range entities {
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
		// 为了不让画面太乱，我们只画 主角(100) 和 眼(200) 看到的物体
		if id == 100 || id == 200 {
			aoiSet := mgr.GetAOI(id)
			aoiSet.ForEach(func(targetID uint32) bool {
				target := entities[targetID]
				if target != nil {
					lineColor := "rgba(24, 144, 255, 0.3)" // 默认蓝色连线
					if id == 200 {
						lineColor = "rgba(82, 196, 26, 0.3)" // 眼的视野是绿色连线
					}

					// 如果是主角(100)看到了目标，但这个目标实际上是在眼(200)的周围
					// 这证明了 Subscribe 功能生效
					if id == 100 {
						// 简单的距离判断来区分这根线是因为主角自己看到的，还是因为订阅看到的
						dist := distance(pos, target.GetPos())
						if dist > GridSize*2*1.414 {
							// 距离很远却能看到，说明是共享视野
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

func distance(p1, p2 *grid.Position) float64 {
	return math.Sqrt(math.Pow(float64(p1.X-p2.X), 2) + math.Pow(float64(p1.Z-p2.Z), 2))
}
