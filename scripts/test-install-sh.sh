#!/bin/sh
set -eu

case "$(uname -s)" in
	Linux) ;;
	*)
		echo "skipping install.sh smoke test outside Linux"
		exit 0
		;;
esac

case "$(uname -m)" in
	x86_64|amd64) arch="amd64" ;;
	arm64|aarch64) arch="arm64" ;;
	*)
		echo "skipping install.sh smoke test on unsupported architecture: $(uname -m)"
		exit 0
		;;
esac

repo_root="$(CDPATH= cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fixture="$tmp/fixture"
fakebin="$tmp/bin"
install_dir="$tmp/install"
home_dir="$tmp/home"
mkdir -p "$fixture/archive" "$fakebin" "$install_dir" "$home_dir"

version="9.8.7"
asset_name="uv-python-hook_${version}_linux_${arch}.tar.gz"
archive="$fixture/$asset_name"
checksums="$fixture/checksums.txt"

printf '%s\n' '#!/bin/sh' 'echo uv-python-hook smoke test' > "$fixture/archive/uv-python-hook"
chmod 755 "$fixture/archive/uv-python-hook"
tar -czf "$archive" -C "$fixture/archive" uv-python-hook

if command -v sha256sum >/dev/null 2>&1; then
	sha="$(sha256sum "$archive" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
	sha="$(shasum -a 256 "$archive" | awk '{print $1}')"
else
	echo "error: required command not found: sha256sum or shasum" >&2
	exit 1
fi
printf '%s  %s\n' "$sha" "$asset_name" > "$checksums"

cat > "$fixture/release.json" <<EOF
{
  "tag_name": "v${version}",
  "assets": [
    { "browser_download_url": "https://example.test/${asset_name}" },
    { "browser_download_url": "https://example.test/checksums.txt" }
  ]
}
EOF

cat > "$fakebin/curl" <<EOF
#!/bin/sh
url=""
dest=""
while [ "\$#" -gt 0 ]; do
	case "\$1" in
		-o)
			shift
			dest="\$1"
			;;
		-*)
			;;
		*)
			url="\$1"
			;;
	esac
	shift
done

case "\$url" in
	*/releases/latest) cp "$fixture/release.json" "\$dest" ;;
	*/$asset_name) cp "$archive" "\$dest" ;;
	*/checksums.txt) cp "$checksums" "\$dest" ;;
	*)
		echo "unexpected url: \$url" >&2
		exit 2
		;;
esac
EOF
chmod 755 "$fakebin/curl"

PATH="$fakebin:$PATH" \
HOME="$home_dir" \
SHELL="/bin/bash" \
UV_PYTHON_HOOK_REPO="example/uv-python-hook" \
UV_PYTHON_HOOK_INSTALL_DIR="$install_dir" \
UV_PYTHON_HOOK_NO_MODIFY_PATH=1 \
	sh "$repo_root/scripts/install.sh"

installed="$install_dir/uv-python-hook"
if [ ! -x "$installed" ]; then
	echo "error: install.sh did not install an executable binary" >&2
	exit 1
fi

if [ -f "$home_dir/.bashrc" ]; then
	echo "error: install.sh modified shell profile despite UV_PYTHON_HOOK_NO_MODIFY_PATH=1" >&2
	exit 1
fi

echo "install.sh Linux smoke test passed"
