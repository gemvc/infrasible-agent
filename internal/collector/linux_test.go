package collector
import ("math";"testing")
func TestSwap(t *testing.T){
 m,e:=memoryFrom("MemTotal: 1000 kB\nMemAvailable: 600 kB\nSwapTotal: 100 kB\nSwapFree: 70 kB\n")
 if e!=nil||m["swap_used"]!=30720{t.Fatalf("swap: %v %v",m,e)}
}
func TestCPUGuestExcluded(t *testing.T){
 a:=[]uint64{100,0,0,100,0,0,0,0,10,0}
 b:=[]uint64{140,0,0,160,0,0,0,0,30,0}
 m,e:=usage(a,b)
 if e!=nil||math.Abs(m["total_pct"]-40)>0.01{t.Fatalf("CPU: %v %v",m,e)}
}
