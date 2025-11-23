package main

import (
	"github.com/beijian128/aoi/aoi"
	"time"
)

func main() {
	m := aoi.NewManager(10, 180, 120)
	m.RegisterLeaveAOIHandler(func(self, other uint32) {
		println("leave", self, other)
	})
	m.RegisterEnterAOIHandler(func(self, other uint32) {
		println("enter", self, other)
	})
	e1 := aoi.NewEntity(1, &aoi.Position{X: 10, Z: 10}, true)
	m.AddEntity(e1)
	m.AddEntity(aoi.NewEntity(2, &aoi.Position{X: 20, Z: 10}, true))
	m.AddEntity(aoi.NewEntity(3, &aoi.Position{X: 30, Z: 10}, false))
	m.AddEntity(aoi.NewEntity(4, &aoi.Position{X: 40, Z: 10}, false))

	for i := 0; i < 100; i++ {
		m.MoveEntity(1, &aoi.Position{X: 10 + float32(i*5), Z: 10})

		//fmt.Println(e1.GetPos(), m.GetAOI(1).Values())
		time.Sleep(time.Millisecond * 1000)

	}
}
