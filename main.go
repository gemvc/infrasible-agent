// Infrasible is a read-only Linux monitoring agent. It writes JSON only to stdout.
package main

import (
 "bufio"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "runtime"
 "strconv"
 "strings"
 "syscall"
 "time"
)

type CPU struct { TotalPct float64 `json:"total_pct"`; UserPct float64 `json:"user_pct"`; SystemPct float64 `json:"system_pct"`; IOWaitPct float64 `json:"iowait_pct"` }
type Memory struct { TotalBytes uint64 `json:"total_bytes"`; AvailableBytes uint64 `json:"available_bytes"`; SwapTotalBytes uint64 `json:"swap_total_bytes"`; SwapUsedBytes uint64 `json:"swap_used_bytes"` }
type Load struct { One float64 `json:"one"`; Five float64 `json:"five"`; Fifteen float64 `json:"fifteen"`; RunnableTasks uint64 `json:"runnable_tasks"`; TotalTasks uint64 `json:"total_tasks"` }
type Filesystem struct { Mount string `json:"mount"`; TotalBytes uint64 `json:"total_bytes"`; AvailableBytes uint64 `json:"available_bytes"` }
type Interface struct { Name string `json:"name"`; RXBytes uint64 `json:"rx_bytes"`; TXBytes uint64 `json:"tx_bytes"`; RXPackets uint64 `json:"rx_packets"`; TXPackets uint64 `json:"tx_packets"`; RXDrops uint64 `json:"rx_drops"`; TXDrops uint64 `json:"tx_drops"` }
type Host struct { CPU CPU `json:"cpu"`; Memory Memory `json:"memory"`; Load Load `json:"load"`; Filesystems []Filesystem `json:"filesystems"`; Interfaces []Interface `json:"interfaces"` }
type Snapshot struct { SchemaVersion string `json:"schema_version"`; AgentVersion string `json:"agent_version"`; CollectedAt string `json:"collected_at"`; IntervalMS int64 `json:"interval_ms"`; Host Host `json:"host"`; Errors []string `json:"errors"` }

func read(path string) (string,error) { b,e:=os.ReadFile(path);return string(b),e }
func numbers(fields []string) ([]uint64,error) { result:=make([]uint64,0,len(fields));for _,s:=range fields {n,e:=strconv.ParseUint(s,10,64);if e!=nil{return nil,e};result=append(result,n)};return result,nil }
func cpuTimes(line string) ([]uint64,error) { f:=strings.Fields(line);if len(f)<8||f[0]!="cpu"{return nil,errors.New("invalid cpu header")};return numbers(f[1:]) }
func cpuSample() ([]uint64,error) { s,e:=read("/proc/stat");if e!=nil{return nil,e};return cpuTimes(strings.SplitN(s,"\n",2)[0]) }
func diff(a,b uint64) uint64 {if b<a{return 0};return b-a}
func percent(n,total uint64) float64 {if total==0{return 0};return float64(n)*100/float64(total)}
func cpuUsage(a,b []uint64) CPU {if len(a)<8||len(b)<8{return CPU{}};var total uint64;for i:=0;i<len(a)&&i<len(b);i++ {total+=diff(a[i],b[i])};idle:=diff(a[3],b[3])+diff(a[4],b[4]);return CPU{TotalPct:percent(total-idle,total),UserPct:percent(diff(a[0],b[0])+diff(a[1],b[1]),total),SystemPct:percent(diff(a[2],b[2]),total),IOWaitPct:percent(diff(a[4],b[4]),total)}}
func memoryFrom(s string) (Memory,error) {m:=map[string]uint64{};for _,line:=range strings.Split(s,"\n"){f:=strings.Fields(line);if len(f)<2{continue};key:=strings.TrimSuffix(f[0],":");n,e:=strconv.ParseUint(f[1],10,64);if e==nil{m[key]=n*1024}};if m["MemTotal"]==0{return Memory{},errors.New("missing MemTotal")};avail:=m["MemAvailable"];if avail==0{avail=m["MemFree"]+m["Buffers"]+m["Cached"]};return Memory{TotalBytes:m["MemTotal"],AvailableBytes:avail,SwapTotalBytes:m["SwapTotal"],SwapUsedBytes:diff(m["SwapFree"],m["SwapTotal"])},nil}
func loadFrom(s string) (Load,error) {f:=strings.Fields(s);if len(f)<4{return Load{},errors.New("invalid loadavg")};a,e:=strconv.ParseFloat(f[0],64);if e!=nil{return Load{},e};b,e:=strconv.ParseFloat(f[1],64);if e!=nil{return Load{},e};c,e:=strconv.ParseFloat(f[2],64);if e!=nil{return Load{},e};tasks:=strings.Split(f[3],"/");if len(tasks)!=2{return Load{},errors.New("invalid tasks")};x,e:=strconv.ParseUint(tasks[0],10,64);if e!=nil{return Load{},e};y,e:=strconv.ParseUint(tasks[1],10,64);return Load{a,b,c,x,y},e}
func interfacesFrom(s string) []Interface {out:=[]Interface{};sc:=bufio.NewScanner(strings.NewReader(s));for sc.Scan(){line:=sc.Text();i:=strings.IndexByte(line,':');if i<0{continue};v,e:=numbers(strings.Fields(line[i+1:]));if e!=nil||len(v)<16{continue};out=append(out,Interface{Name:strings.TrimSpace(line[:i]),RXBytes:v[0],TXBytes:v[8],RXPackets:v[1],TXPackets:v[9],RXDrops:v[3],TXDrops:v[11]})};return out}
func filesystems() []Filesystem {out:=[]Filesystem{};f,e:=os.Open("/proc/mounts");if e!=nil{return out};defer f.Close();seen:=map[string]bool{};sc:=bufio.NewScanner(f);for sc.Scan(){v:=strings.Fields(sc.Text());if len(v)<3{continue};mount:=strings.ReplaceAll(v[1],"\\040"," ");if seen[mount]||strings.HasPrefix(mount,"/proc")||strings.HasPrefix(mount,"/sys")||strings.HasPrefix(mount,"/dev"){continue};switch v[2]{case "proc","sysfs","tmpfs","devtmpfs","cgroup","cgroup2","overlay","squashfs","tracefs","securityfs","debugfs":continue};var st syscall.Statfs_t;if syscall.Statfs(mount,&st)!=nil{continue};seen[mount]=true;out=append(out,Filesystem{mount,st.Blocks*uint64(st.Bsize),st.Bavail*uint64(st.Bsize)})};return out}
func snapshot(wait time.Duration) Snapshot {s:=Snapshot{SchemaVersion:"1.0",AgentVersion:agentVersion,Errors:[]string{}};a,e:=cpuSample();if e!=nil{s.Errors=append(s.Errors,"cpu: "+e.Error())};start:=time.Now();time.Sleep(wait);b,e:=cpuSample();if e!=nil{s.Errors=append(s.Errors,"cpu: "+e.Error())};if len(a)>0&&len(b)>0{s.Host.CPU=cpuUsage(a,b)};s.IntervalMS=time.Since(start).Milliseconds();if x,e:=read("/proc/meminfo");e==nil{if m,e:=memoryFrom(x);e==nil{s.Host.Memory=m}else{s.Errors=append(s.Errors,"memory: "+e.Error())}}else{s.Errors=append(s.Errors,"memory: "+e.Error())};if x,e:=read("/proc/loadavg");e==nil{if l,e:=loadFrom(x);e==nil{s.Host.Load=l}else{s.Errors=append(s.Errors,"load: "+e.Error())}}else{s.Errors=append(s.Errors,"load: "+e.Error())};if x,e:=read("/proc/net/dev");e==nil{s.Host.Interfaces=interfacesFrom(x)}else{s.Errors=append(s.Errors,"network: "+e.Error())};s.Host.Filesystems=filesystems();s.CollectedAt=time.Now().UTC().Format(time.RFC3339Nano);return s}
func main(){
 if runtime.GOOS!="linux"{fmt.Fprintln(os.Stderr,"Linux only");os.Exit(2)}
 if len(os.Args)==3 && os.Args[1]=="version" && os.Args[2]=="--json" {
  if err:=writeVersion(os.Stdout);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
  return
 }
 if len(os.Args)==3 && os.Args[1]=="snapshot" && os.Args[2]=="--json" {
  if err:=json.NewEncoder(os.Stdout).Encode(snapshot(time.Second));err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
  return
 }
 if len(os.Args)==3 && os.Args[1]=="stream" && os.Args[2]=="--json" {
  encoder:=json.NewEncoder(os.Stdout)
  for {
   if err:=encoder.Encode(snapshot(time.Second));err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
  }
 }
 fmt.Fprintln(os.Stderr,"usage: infrasible-agent version|snapshot|stream --json")
 os.Exit(2)
}
