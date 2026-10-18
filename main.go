// Command infrasible-agent is a Linux-only unattended host monitoring daemon.
// It exposes no HTTP management listener; operator maintenance uses local CLI
// commands over an existing authorized SSH/Ansible connection.
package main

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "log/slog"
 "os"
 "os/signal"
 "runtime"
 "strings"
 "syscall"
 "time"

 "github.com/gemvc/infrasible-agent/internal/agent"
 "github.com/gemvc/infrasible-agent/internal/config"
 "github.com/gemvc/infrasible-agent/internal/storage"
)
func usage()error{return errors.New("usage: infrasible-agent run|config validate|health status|storage status|storage cleanup --dry-run|storage cleanup --safe [--config=PATH]")}
func run(args []string)error{
 if runtime.GOOS!="linux"{return errors.New("Linux only")}
 configPath:="/etc/infrasible-agent/config.json"
 kept:=make([]string,0,len(args))
 for _,arg:=range args {
  if strings.HasPrefix(arg,"--config="){configPath=strings.TrimPrefix(arg,"--config=")}else{kept=append(kept,arg)}
 }
 args=kept
 if len(args)==0{return usage()}
 c,e:=config.Load(configPath);if e!=nil{return e}
 if len(args)==2&&args[0]=="config"&&args[1]=="validate"{
  return json.NewEncoder(os.Stdout).Encode(map[string]any{"valid":true,"config_version":c.ConfigVersion})
 }
 if c.DataPath==""{return errors.New("missing storage path")}
 if len(args)==1&&args[0]=="run"{
  s,e:=storage.Open(c.DataPath);if e!=nil{return e};defer s.Close()
  log:=slog.New(slog.NewJSONHandler(os.Stderr,nil))
  ctx,stop:=signal.NotifyContext(context.Background(),syscall.SIGTERM,syscall.SIGINT)
  defer stop()
  return agent.New(c,s,log).Run(ctx)
 }
 if len(args)==2&&args[0]=="health"&&args[1]=="status"{
  s,e:=storage.Open(c.DataPath);if e!=nil{return e};defer s.Close()
  ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
  h,e:=s.Health(ctx,time.Now());if e!=nil{return e}
  return json.NewEncoder(os.Stdout).Encode(h)
 }
 if len(args)>=2&&args[0]=="storage"{
  s,e:=storage.Open(c.DataPath);if e!=nil{return e};defer s.Close()
  ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
  switch {
  case len(args)==2&&args[1]=="status":
   st,e:=s.Status(ctx,time.Now());if e!=nil{return e};return json.NewEncoder(os.Stdout).Encode(st)
  case len(args)==3&&args[1]=="cleanup"&&(args[2]=="--dry-run"||args[2]=="--safe"):
   result,e:=s.Cleanup(ctx,time.Now(),args[2]=="--dry-run",500);if e!=nil{return e}
   return json.NewEncoder(os.Stdout).Encode(result)
  }
 }
 return usage()
}
func main(){if e:=run(os.Args[1:]);e!=nil{fmt.Fprintln(os.Stderr,e);os.Exit(1)}}
