package main

import (
	"github.com/beijian128/aoi/aoi"
	"github.com/sirupsen/logrus"
	"time"
)

func main() {
	m := aoi.NewManager(10, 180, 120)
	m.RegisterLeaveAOIHandler(func(self, other uint32) {
		//println("leave", self, other)
	})
	m.RegisterEnterAOIHandler(func(self, other uint32) {
		logrus.Infof("enter %d %d", self, other)
	})
	e1 := aoi.NewEntity(1, &aoi.Position{X: 10, Z: 10}, true)
	m.AddEntity(e1)
	m.AddEntity(aoi.NewEntity(2, &aoi.Position{X: 20, Z: 10}, true))
	m.AddEntity(aoi.NewEntity(3, &aoi.Position{X: 30, Z: 10}, false))
	m.AddEntity(aoi.NewEntity(4, &aoi.Position{X: 40, Z: 10}, false))

	//for i := 0; i < 100000; i++ {
	//	st := time.Now().UnixMilli()
	//	m.MoveEntity(1, &aoi.Position{X: 15, Z: 10})
	//	d := float32(time.Now().UnixMilli()-st) / 1000.
	//	m.OnTick(d)
	//	//fmt.Println(e1.GetPos(), m.GetAOI(1).Values())
	//	time.Sleep(time.Millisecond * time.Duration(rand.Int63n(3000)))
	//	m.MoveEntity(1, &aoi.Position{X: 2, Z: 10})
	//
	//	time.Sleep(time.Millisecond * time.Duration(rand.Int63n(3000)))
	//	d = float32(time.Now().UnixMilli()-st) / 1000.
	//	m.OnTick(d)
	//}
	start := time.Now().UnixNano()
	timer := time.NewTicker(time.Millisecond * 60)
	timer2 := time.NewTicker(time.Second * 2)
	x := 1
	for {
		select {
		case <-timer.C:
			now := time.Now().UnixNano()
			m.OnTick(float32(now-start) / 1e9)
			start = now
		case <-timer2.C:
			x++
			if x%2 == 0 {
				m.MoveEntity(1, &aoi.Position{X: 15, Z: 10})
			} else {
				m.MoveEntity(1, &aoi.Position{X: 2, Z: 10})
			}

		}
	}
}
