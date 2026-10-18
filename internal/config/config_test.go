package config
import "testing"
func TestRejectUntrusted(t *testing.T){
 cases:=[]Config{
 {ConfigVersion:2},
 {ConfigVersion:1,MonitoringServers:[]Server{{ID:"a",LiveURL:"ws://evil",ServerName:"host",CAFile:"ca",ClientCertFile:"cert",ClientKeyFile:"key"}}},
 {ConfigVersion:1,MonitoringServers:[]Server{{ID:"a",LiveURL:"wss://host",ServerName:"host",CAFile:"ca",ClientCertFile:"cert",ClientKeyFile:"key"},{ID:"a",LiveURL:"wss://host",ServerName:"host",CAFile:"ca",ClientCertFile:"cert",ClientKeyFile:"key"}}},
 }
 for _,c:=range cases {if e:=c.Validate();e==nil{t.Fatalf("accepted %+v",c)}}
}
