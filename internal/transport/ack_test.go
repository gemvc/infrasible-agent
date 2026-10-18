package transport

import (
 "testing"
 "github.com/gemvc/infrasible-agent/internal/storage"
)
func TestDurableACKMustMatchAllFields(t *testing.T){
 p:=storage.Pending{RecordID:"id",Hash:"sha",PeriodStartMS:900000}
 valid:=ACK{Durable:true,AgentID:"agent",RecordID:"id",PayloadSHA256:"sha",PeriodStartMS:900000,PersistenceDomain:"primary",SchemaVersion:2}
 if e:=validateACK(valid,p,"agent","primary");e!=nil{t.Fatal(e)}
 invalid:=[]struct{name string;ack ACK}{
 {"not durable",func()ACK{a:=valid;a.Durable=false;return a}()},
 {"different agent",func()ACK{a:=valid;a.AgentID="other";return a}()},
 {"different record",func()ACK{a:=valid;a.RecordID="other";return a}()},
 {"different hash",func()ACK{a:=valid;a.PayloadSHA256="other";return a}()},
 {"different period",func()ACK{a:=valid;a.PeriodStartMS=0;return a}()},
 {"different domain",func()ACK{a:=valid;a.PersistenceDomain="secondary";return a}()},
 {"different schema",func()ACK{a:=valid;a.SchemaVersion=1;return a}()},
 }
 for _,tc:=range invalid{t.Run(tc.name,func(t *testing.T){if e:=validateACK(tc.ack,p,"agent","primary");e==nil{t.Fatal("invalid ACK accepted")}})}
}
