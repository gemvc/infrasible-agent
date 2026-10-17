package main

import (
 "bytes"
 "encoding/json"
 "testing"
)

func TestVersionInfo(t *testing.T) {
 v := versionInfo()
 if v.AgentVersion != "0.1.0" || v.ProtocolMajor != 1 || v.ProtocolMinor != 0 || v.SchemaVersion != "1.0" {
  t.Fatalf("unexpected version contract: %+v", v)
 }
 if v.OS == "" || v.Arch == "" || len(v.Capabilities) != 2 {
  t.Fatalf("incomplete platform/capabilities: %+v", v)
 }
 var b bytes.Buffer
 if err := writeVersion(&b); err != nil { t.Fatal(err) }
 var decoded VersionInfo
 if err := json.Unmarshal(b.Bytes(), &decoded); err != nil { t.Fatal(err) }
 if decoded.AgentVersion != v.AgentVersion || decoded.Capabilities[1] != "stream" { t.Fatalf("unexpected JSON: %s", b.String()) }
}
