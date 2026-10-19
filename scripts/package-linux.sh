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
  cp packaging/infrasible-agent.service "$dir/infrasible-agent.service"
  cp packaging/config.example.json "$dir/config.example.json"
  tar -C "$dir" -czf "dist/infrasible-agent_${VERSION}_linux_${target}.tar.gz" infrasible-agent LICENSE infrasible-agent.service config.example.json
  rm -rf "$dir"
  if [[ -n "$debarch" ]] && command -v dpkg-deb >/dev/null 2>&1; then
    pkg="$(mktemp -d)"
    install -Dm755 "dist/$binary" "$pkg/usr/bin/infrasible-agent"
    mkdir -p "$pkg/DEBIAN" "$pkg/usr/share/doc/infrasible-agent"
    cp LICENSE "$pkg/usr/share/doc/infrasible-agent/copyright"
    install -Dm644 packaging/infrasible-agent.service "$pkg/usr/lib/systemd/system/infrasible-agent.service"
    install -Dm644 packaging/config.example.json "$pkg/usr/share/doc/infrasible-agent/config.example.json"
    install -Dm755 packaging/debian/postinst "$pkg/DEBIAN/postinst"
    install -Dm755 packaging/debian/prerm "$pkg/DEBIAN/prerm"
    install -Dm755 packaging/debian/postrm "$pkg/DEBIAN/postrm"
    printf 'Package: infrasible-agent\nVersion: %s\nSection: admin\nPriority: optional\nArchitecture: %s\nMaintainer: Infrasible Maintainers <maintainers@users.noreply.github.com>\nDescription: Linux monitoring daemon with local SQLite\n' "$VERSION" "$debarch" > "$pkg/DEBIAN/control"
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
Summary: Linux host monitoring daemon
License: MIT
BuildArch: x86_64
%description
Infrasible Linux monitoring daemon with outbound mTLS channels and no inbound API.
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
# Hash only shippable artifacts. Do NOT glob infrasible-agent-linux-* — that also
# matches *.sha256 sidecars and breaks `sha256sum -c SHA256SUMS` on CI.
(
  cd dist
  shopt -s nullglob
  files=(
    infrasible-agent-linux-amd64
    infrasible-agent-linux-arm64
    infrasible-agent-linux-armv7
    infrasible-agent-linux-armv6
    *.tar.gz
    *.deb
    *.rpm
  )
  sha256sum "${files[@]}" | awk '!seen[$2]++' > SHA256SUMS
)
# Preserve raw-hash sidecars consumed by the existing Infrasible desktop client.
# Sidecars are outputs only; they are never entries inside SHA256SUMS.
for target in amd64 arm64 armv7 armv6; do
  sha256sum "dist/infrasible-agent-linux-$target" | awk '{print $1}' > "dist/infrasible-agent-linux-$target.sha256"
done
echo "Packaged agent $VERSION"
