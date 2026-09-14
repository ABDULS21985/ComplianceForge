#!/bin/sh

set -eu

# Release archives and hashes are pinned so a compromised mutable action/tag
# cannot silently replace a scanner in CI.
TRIVY_VERSION=0.74.0
SYFT_VERSION=1.51.1
GITLEAKS_VERSION=8.30.1
GOSEC_VERSION=2.28.0
GOVULNCHECK_VERSION=v1.1.4

tools_dir=${TOOLS_DIR:-${RUNNER_TEMP:-.cache}/complianceforge-security-tools/bin}
mkdir -p "$tools_dir"
tools_dir=$(CDPATH= cd -- "$tools_dir" && pwd)

os=$(uname -s)
arch=$(uname -m)

case "$os/$arch" in
  Linux/x86_64|Linux/amd64)
    trivy_asset="trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz"
    trivy_sha=2ae6fe3ee734b7fdf11335663e18c75ea12dccc76062f09f164a3b0f8be4371a
    syft_asset="syft_${SYFT_VERSION}_linux_amd64.tar.gz"
    syft_sha=8fcb33017a0dc1058298c923c436d19dfa68ae93968e0b423248542e3afb9fc3
    gitleaks_asset="gitleaks_${GITLEAKS_VERSION}_linux_x64.tar.gz"
    gitleaks_sha=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb
    gosec_asset="gosec_${GOSEC_VERSION}_linux_amd64.tar.gz"
    gosec_sha=d7882e505b1ff345d458bf0e893eec8019bc849f861ad73a212869540dd505ff
    ;;
  Linux/aarch64|Linux/arm64)
    trivy_asset="trivy_${TRIVY_VERSION}_Linux-ARM64.tar.gz"
    trivy_sha=b94ce1976bbf3c15b514b605ee88be7c6d94a29be2302847ff01cb794d47aad5
    syft_asset="syft_${SYFT_VERSION}_linux_arm64.tar.gz"
    syft_sha=a7fd2b784e6664acd44719270574f6cd8c6864fc2b1700bf9099bd1cccda7d7f
    gitleaks_asset="gitleaks_${GITLEAKS_VERSION}_linux_arm64.tar.gz"
    gitleaks_sha=e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080
    gosec_asset="gosec_${GOSEC_VERSION}_linux_arm64.tar.gz"
    gosec_sha=63259681b6e4b9e7a24d4e187b485e75d3844d28d512b0c97dc831e51d374720
    ;;
  Darwin/x86_64|Darwin/amd64)
    trivy_asset="trivy_${TRIVY_VERSION}_macOS-64bit.tar.gz"
    trivy_sha=472816f6888dda689d075c30254d4210b4d1035acf365aa72332f584c2f60485
    syft_asset="syft_${SYFT_VERSION}_darwin_amd64.tar.gz"
    syft_sha=0e186ce1d4351ec276126851ca3ff258ed070e93e73574ed64858d4fc2339867
    gitleaks_asset="gitleaks_${GITLEAKS_VERSION}_darwin_x64.tar.gz"
    gitleaks_sha=dfe101a4db2255fc85120ac7f3d25e4342c3c20cf749f2c20a18081af1952709
    gosec_asset="gosec_${GOSEC_VERSION}_darwin_amd64.tar.gz"
    gosec_sha=ad23af3a6bfef8112a2da386acd61ede1374c8d022c06d8ef130ccf9748311d4
    ;;
  Darwin/arm64|Darwin/aarch64)
    trivy_asset="trivy_${TRIVY_VERSION}_macOS-ARM64.tar.gz"
    trivy_sha=1caada5e0e2091909357c7525d3aa76f4b660b13821bc143b190c7483e31cc11
    syft_asset="syft_${SYFT_VERSION}_darwin_arm64.tar.gz"
    syft_sha=ac063af3b9874769deb7ea1e6d76841e68f9e3bb50cd654226fc977de65532c1
    gitleaks_asset="gitleaks_${GITLEAKS_VERSION}_darwin_arm64.tar.gz"
    gitleaks_sha=b40ab0ae55c505963e365f271a8d3846efbc170aa17f2607f13df610a9aeb6a5
    gosec_asset="gosec_${GOSEC_VERSION}_darwin_arm64.tar.gz"
    gosec_sha=6c4993a0ab5e3007d66c87cbcb4e3948f8000971f8eeaf3ac269cbc87a603ba4
    ;;
  *)
    printf 'unsupported security-tool platform: %s/%s\n' "$os" "$arch" >&2
    exit 1
    ;;
esac

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/complianceforge-tools.XXXXXX")
cleanup() {
  if [ -d "$work_dir" ]; then
    find "$work_dir" -mindepth 1 -delete
    rmdir "$work_dir"
  fi
}
trap cleanup EXIT HUP INT TERM

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

install_archive() {
  name=$1
  url=$2
  expected_sha=$3
  archive="$work_dir/$name.tar.gz"
  extract_dir="$work_dir/$name"

  mkdir -p "$extract_dir"
  curl --fail --location --proto '=https' --tlsv1.2 --silent --show-error \
    --retry 3 --retry-all-errors --output "$archive" "$url"
  actual_sha=$(sha256_file "$archive")
  if [ "$actual_sha" != "$expected_sha" ]; then
    printf '%s checksum mismatch: expected %s, got %s\n' "$name" "$expected_sha" "$actual_sha" >&2
    exit 1
  fi

  tar -xzf "$archive" -C "$extract_dir"
  binary=$(find "$extract_dir" -type f -name "$name" -print | head -n 1)
  [ -n "$binary" ] || {
    printf '%s release did not contain the expected binary\n' "$name" >&2
    exit 1
  }
  install -m 0755 "$binary" "$tools_dir/$name"
}

install_archive trivy \
  "https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/${trivy_asset}" \
  "$trivy_sha"
install_archive syft \
  "https://github.com/anchore/syft/releases/download/v${SYFT_VERSION}/${syft_asset}" \
  "$syft_sha"
install_archive gitleaks \
  "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/${gitleaks_asset}" \
  "$gitleaks_sha"
install_archive gosec \
  "https://github.com/securego/gosec/releases/download/v${GOSEC_VERSION}/${gosec_asset}" \
  "$gosec_sha"

# govulncheck has no binary release archive. A fixed module version is built
# under Go's checksum-database verification without modifying this module.
GOBIN="$tools_dir" GOTOOLCHAIN=local go install "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"

if [ -n "${GITHUB_PATH:-}" ]; then
  printf '%s\n' "$tools_dir" >> "$GITHUB_PATH"
fi

printf 'Installed pinned security tools in %s\n' "$tools_dir"
"$tools_dir/trivy" --version
"$tools_dir/syft" version
"$tools_dir/gitleaks" version
"$tools_dir/gosec" -version
"$tools_dir/govulncheck" -version
