#!/bin/sh
set -eu

repo="${UV_PYTHON_HOOK_REPO:-jinxiao/python-uv-hooks-4-ai}"
install_dir="${UV_PYTHON_HOOK_INSTALL_DIR:-"$HOME/.local/bin"}"
api_url="https://api.github.com/repos/$repo/releases/latest"

need() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "error: required command not found: $1" >&2
		exit 1
	fi
}

download() {
	url="$1"
	dest="$2"
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$url" -o "$dest"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$url" -O "$dest"
	else
		echo "error: required command not found: curl or wget" >&2
		exit 1
	fi
}

case "$(uname -s)" in
	Linux) os="linux" ;;
	Darwin) os="darwin" ;;
	*) echo "error: this installer supports Linux and macOS only" >&2; exit 1 ;;
esac

case "$(uname -m)" in
	x86_64|amd64) arch="amd64" ;;
	arm64|aarch64) arch="arm64" ;;
	*) echo "error: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

need grep
need mktemp
need sed
need tar

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

release_json="$tmp/release.json"
download "$api_url" "$release_json"

tag="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$release_json" | head -n 1)"
if [ -z "$tag" ]; then
	echo "error: could not determine latest release tag from $api_url" >&2
	exit 1
fi

version="${tag#v}"
asset_name="uv-python-hook_${version}_${os}_${arch}.tar.gz"
asset_url="$(grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*"' "$release_json" | sed 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"//;s/"$//' | grep "/$asset_name$" | head -n 1)"
checksums_url="$(grep -o '"browser_download_url"[[:space:]]*:[[:space:]]*"[^"]*"' "$release_json" | sed 's/.*"browser_download_url"[[:space:]]*:[[:space:]]*"//;s/"$//' | grep "/checksums.txt$" | head -n 1)"

if [ -z "$asset_url" ]; then
	echo "error: release $tag does not contain $asset_name" >&2
	exit 1
fi
if [ -z "$checksums_url" ]; then
	echo "error: release $tag does not contain checksums.txt" >&2
	exit 1
fi

archive="$tmp/$asset_name"
checksums="$tmp/checksums.txt"
download "$asset_url" "$archive"
download "$checksums_url" "$checksums"

expected="$(grep "[[:space:]]$asset_name\$" "$checksums" | awk '{print $1}' | head -n 1)"
if [ -z "$expected" ]; then
	echo "error: checksums.txt does not contain $asset_name" >&2
	exit 1
fi

if command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "$archive" | awk '{print $1}')"
else
	echo "error: required command not found: shasum or sha256sum" >&2
	exit 1
fi

if [ "$expected" != "$actual" ]; then
	echo "error: checksum mismatch for $asset_name" >&2
	exit 1
fi

tar -xzf "$archive" -C "$tmp"
if [ ! -f "$tmp/uv-python-hook" ]; then
	echo "error: archive did not contain uv-python-hook" >&2
	exit 1
fi

mkdir -p "$install_dir"
cp "$tmp/uv-python-hook" "$install_dir/uv-python-hook"
chmod 755 "$install_dir/uv-python-hook"

path_line="export PATH=\"$install_dir:\$PATH\""
profile=""
case "${SHELL:-}" in
	*/zsh) profile="$HOME/.zshrc" ;;
	*/bash) profile="$HOME/.bashrc" ;;
	*/fish) profile="$HOME/.config/fish/config.fish" ;;
esac

if [ "${UV_PYTHON_HOOK_NO_MODIFY_PATH:-}" != "1" ]; then
	if printf '%s' "$PATH" | tr ':' '\n' | grep -Fx "$install_dir" >/dev/null 2>&1; then
		:
	elif [ -n "$profile" ]; then
		mkdir -p "$(dirname "$profile")"
		if [ "${profile##*/}" = "config.fish" ]; then
			path_line="fish_add_path \"$install_dir\""
		fi
		if [ ! -f "$profile" ] || ! grep -F "$install_dir" "$profile" >/dev/null 2>&1; then
			{
				echo ""
				echo "# uv-python-hook"
				echo "$path_line"
			} >> "$profile"
			echo "Updated PATH in $profile"
		fi
	else
		echo "Add $install_dir to PATH to run uv-python-hook from a new shell."
	fi
fi

echo "Installed uv-python-hook $version to $install_dir/uv-python-hook"
