package main

import (
 "math"
 "strings"
 "testing"
)

func TestCPUUsage(t *testing.T) {
 a,_:=cpuTimes("cpu 100 0 50 850 0 0 0 0 0 0")
 b,_:=cpuTimes("cpu 130 0 60 910 0 0 0 0 0 0")
 v:=cpuUsage(a,b)
 if math.Abs(v.TotalPct-40)>0.001 {t.Fatalf("expected 40 percent, got %v",v.TotalPct)}
}
func TestMemory(t *testing.T) {
 v,e:=memoryFrom("MemTotal: 1000 kB\nMemAvailable: 600 kB\nSwapTotal: 100 kB\nSwapFree: 70 kB\n")
 if e!=nil||v.TotalBytes!=1024000||v.AvailableBytes!=614400||v.SwapUsedBytes!=30720 {t.Fatalf("%+v %v",v,e)}
}
func TestLoad(t *testing.T) {
 v,e:=loadFrom("0.10 0.20 0.30 2/150 12345")
 if e!=nil||v.RunnableTasks!=2||v.TotalTasks!=150 {t.Fatalf("%+v %v",v,e)}
}
func TestInterface(t *testing.T) {
 raw:="Inter-| Receive | Transmit\n face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n  eth0: 1000 10 0 2 0 0 0 0 2000 20 0 3 0 0 0 0\n"
 got:=interfacesFrom(raw)
 if len(got)!=1||got[0].Name!="eth0"||got[0].RXDrops!=2||got[0].TXDrops!=3 {t.Fatalf("%+v",got)}
}
func TestInvalidCPU(t *testing.T) {
 if _,e:=cpuTimes(strings.TrimSpace("cpu 1"));e==nil{t.Fatal("expected error")}
}
