package config

import (
 "encoding/json"
 "errors"
 "fmt"
 "net/url"
 "os"
 "strings"
 "time"
)

type Server struct {
 ID string `json:"id"`
 LiveURL string `json:"live_url"`
 PersistenceURL string `json:"persistence_url"`
 PersistenceDomain string `json:"persistence_domain"`
 ServerName string `json:"server_name"`
 CAFile string `json:"ca_file"`
 ClientCertFile string `json:"client_cert_file"`
 ClientKeyFile string `json:"client_key_file"`
}
type Config struct {
 ConfigVersion int `json:"config_version"`
 DataPath string `json:"data_path"`
 MonitoringServers []Server `json:"monitoring_servers"`
}
func Load(path string) (Config,error) {
 f,e:=os.Open(path); if e!=nil{return Config{},e};defer f.Close()
 d:=json.NewDecoder(f);d.DisallowUnknownFields()
 var c Config
 if e=d.Decode(&c);e!=nil{return Config{},fmt.Errorf("decode config: %w",e)}
 var trailing any
 if e=d.Decode(&trailing);!errors.Is(e,os.ErrClosed) && e==nil{return Config{},errors.New("trailing JSON content")} else if e!=nil && e.Error()!="EOF" {return Config{},fmt.Errorf("trailing JSON: %w",e)}
 if c.DataPath=="" {c.DataPath="/var/lib/infrasible-agent/agent.db"}
 return c,c.Validate()
}
func (c Config) Validate() error {
 if c.ConfigVersion!=1{return errors.New("unsupported config_version")}
 if strings.TrimSpace(c.DataPath)=="" {return errors.New("empty data_path")}
 seen:=map[string]bool{}
 for _,s:=range c.MonitoringServers {
  if s.ID==""||seen[s.ID] {return fmt.Errorf("empty or duplicate monitoring server id: %q",s.ID)}
  seen[s.ID]=true
  if s.LiveURL=="" && s.PersistenceURL=="" {return fmt.Errorf("%s has no channel",s.ID)}
  if s.ServerName==""||s.CAFile==""||s.ClientCertFile==""||s.ClientKeyFile=="" {return fmt.Errorf("%s missing TLS identity",s.ID)}
  for _,entry:=range []struct{raw,scheme string}{{s.LiveURL,"wss"},{s.PersistenceURL,"https"}}{
   if entry.raw=="" {continue}
   u,e:=url.Parse(entry.raw)
   if e!=nil||u.Scheme!=entry.scheme||u.Host==""||u.User!=nil||u.Fragment!="" {return fmt.Errorf("invalid secured endpoint for %s",s.ID)}
  }
  if s.PersistenceURL!="" && s.PersistenceDomain=="" {return fmt.Errorf("%s missing persistence domain",s.ID)}
 }
 return nil
}
func Interval() time.Duration{return time.Second}
