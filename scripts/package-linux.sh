#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
VERSION="$(sed -n 's/^const agentVersion = "\([^"]*\)"/\1/p' version.go)"
[[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "Invalid agent version" >&2; exit 1; }
if [[ -n "${GITHUB_REF_NAME:-}" && "${GITHUB_REF_TYPE:-}" == "tag" ]]; then
  [[ "$GITHUB_REF_NAME" == "v$VERSION" ]] || { echo "Tag does not match agent version" >&2; exit 1; }
fi
for target in amd64 arm64 armv7 armv6; do
  case "$target" in
    amd64) arch=amd64; goarm=''; debarch=amd64 ;;
    arm64) arch=arm64; goarm=''; debarch=arm64 ;;
    armv7) arch=arm; goarm=7; debarch=armhf ;;
    armv6) arch=arm; goarm=6; debarch='' ;;
  esac
  binary="infrasible-agent-linux-$target"
  echo "Building $binary"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOARM="$goarm" GOTOOLCHAIN=local \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o "dist/$binary" .
  file "dist/$binary"
  case "$target" in
    amd64|arm64) [[ "$(file -b "dist/$binary")" == *"ELF 64-bit"* ]] || exit 1 ;;
    armv*) [[ "$(file -b "dist/$binary")" == *"ELF 32-bit"* ]] || exit 1 ;;
  esac
  dir="$(mktemp -d)"
  install -Dm755 "dist/$binary" "$dir/infrasible-agent"
  cp LICENSE "$dir/LICENSE"
  tar -C "$dir" -czf "dist/infrasible-agent_${VERSION}_linux_${target}.tar.gz" infrasible-agent LICENSE
  rm -rf "$dir"
  if [[ -n "$debarch" ]] && command -v dpkg-deb >/dev/null 2>&1; then
    pkg="$(mktemp -d)"
    install -Dm755 "dist/$binary" "$pkg/usr/bin/infrasible-agent"
    mkdir -p "$pkg/DEBIAN" "$pkg/usr/share/doc/infrasible-agent"
    cp LICENSE "$pkg/usr/share/doc/infrasible-agent/copyright"
    printf 'Package: infrasible-agent\nVersion: %s\nSection: admin\nPriority: optional\nArchitecture: %s\nMaintainer: Infrasible Maintainers <maintainers@users.noreply.github.com>\nDescription: Read-only Linux host monitoring agent\n' "$VERSION" "$debarch" > "$pkg/DEBIAN/control"
    dpkg-deb --build --root-owner-group "$pkg" "dist/infrasible-agent_${VERSION}_${debarch}.deb"
    rm -rf "$pkg"
  fi
done
# A generic, service-free RPM for x86_64. Other RPM distributions can use tar.gz.
if command -v rpmbuild >/dev/null 2>&1; then
  top="$(mktemp -d)"
  mkdir -p "$top"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
  cp dist/infrasible-agent-linux-amd64 "$top/SOURCES/infrasible-agent"
  cat > "$top/SPECS/infrasible-agent.spec" <<EOF
Name: infrasible-agent
Version: $VERSION
Release: 1
Summary: Read-only Linux host monitoring agent
License: MIT
BuildArch: x86_64
%description
Infrasible Linux monitoring agent. No background service or network listener.
%prep
%build
%install
mkdir -p %{buildroot}/usr/bin
install -m 0755 %{_sourcedir}/infrasible-agent %{buildroot}/usr/bin/infrasible-agent
%files
/usr/bin/infrasible-agent
EOF
  rpmbuild -bb --define "_topdir $top" "$top/SPECS/infrasible-agent.spec"
  find "$top/RPMS" -type f -name '*.rpm' -exec cp {} dist/ \;
  rm -rf "$top"
fi
( cd dist && sha256sum infrasible-agent-linux-* *.tar.gz *.deb *.rpm 2>/dev/null | awk '!seen[$2]++' > SHA256SUMS )
# Preserve raw-hash sidecars consumed by the existing Infrasible desktop client.
for target in amd64 arm64 armv7 armv6; do
  sha256sum "dist/infrasible-agent-linux-$target" | awk '{print $1}' > "dist/infrasible-agent-linux-$target.sha256"
done
echo "Packaged agent $VERSION"
