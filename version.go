package main

import (
 "encoding/json"
 "io"
 "runtime"
)

const agentVersion = "0.1.0"
const protocolMajor = 1
const protocolMinor = 0

type VersionInfo struct {
 AgentVersion string `json:"agent_version"`
 ProtocolMajor int `json:"protocol_major"`
 ProtocolMinor int `json:"protocol_minor"`
 SchemaVersion string `json:"schema_version"`
 OS string `json:"os"`
 Arch string `json:"arch"`
 Capabilities []string `json:"capabilities"`
}

func versionInfo() VersionInfo {
 return VersionInfo{
  AgentVersion: agentVersion,
  ProtocolMajor: protocolMajor,
  ProtocolMinor: protocolMinor,
  SchemaVersion: "1.0",
  OS: runtime.GOOS,
  Arch: runtime.GOARCH,
  Capabilities: []string{"snapshot", "stream"},
 }
}

func writeVersion(w io.Writer) error {
 return json.NewEncoder(w).Encode(versionInfo())
}
