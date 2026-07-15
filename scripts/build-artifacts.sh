#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

GO=${GO:-go}
NPM=${NPM:-npm}
TARGET_GOOS=${TARGET_GOOS:-linux}
TARGET_GOARCH=${TARGET_GOARCH:-amd64}
VERSION=${VERSION:-$(git describe --tags --always --dirty)}
ARTIFACT_VERSION=$(printf '%s' "$VERSION" | tr -c 'A-Za-z0-9._-' '-')
ARTIFACT_DIR=${ARTIFACT_DIR:-$ROOT/artifacts}

ANDROID_SIGNING_READY=0
if [[ -n "${ANDROID_KEYSTORE_BASE64:-}" && -n "${ANDROID_KEYSTORE_PASSWORD:-}" && -n "${ANDROID_KEY_ALIAS:-}" && -n "${ANDROID_KEY_PASSWORD:-}" ]]; then
  ANDROID_SIGNING_READY=1
elif [[ "${REQUIRE_SIGNED_ANDROID:-0}" == "1" ]]; then
  echo "Release signing is required; configure the four ANDROID_* signing secrets" >&2
  exit 1
elif [[ -n "${ANDROID_KEYSTORE_BASE64:-}${ANDROID_KEYSTORE_PASSWORD:-}${ANDROID_KEY_ALIAS:-}${ANDROID_KEY_PASSWORD:-}" ]]; then
  echo "Ignoring incomplete Android signing configuration; building a debug APK" >&2
fi

mkdir -p "$ARTIFACT_DIR"
ARTIFACT_DIR=$(cd "$ARTIFACT_DIR" && pwd)
WORK_DIR=$(mktemp -d)
trap 'rm -rf "$WORK_DIR"' EXIT

SERVER_ARCHIVE="sim7600d-server-${TARGET_GOOS}-${TARGET_GOARCH}-${ARTIFACT_VERSION}.tar.gz"
WEB_ARCHIVE="sim7600d-web-${ARTIFACT_VERSION}.tar.gz"
BUNDLE_ARCHIVE="sim7600d-server-web-${TARGET_GOOS}-${TARGET_GOARCH}-${ARTIFACT_VERSION}.tar.gz"
ANDROID_APK="sim7600d-android-${ARTIFACT_VERSION}.apk"
CHECKSUMS="SHA256SUMS-${ARTIFACT_VERSION}.txt"

echo "Building server-only ${TARGET_GOOS}/${TARGET_GOARCH} artifact"
mkdir -p "$WORK_DIR/server"
CGO_ENABLED=0 GOOS="$TARGET_GOOS" GOARCH="$TARGET_GOARCH" "$GO" build \
  -trimpath -tags headless -ldflags "-s -w -X main.Version=$VERSION" \
  -o "$WORK_DIR/server/sim7600d" ./cmd/sim7600d
tar -C "$WORK_DIR/server" -czf "$ARTIFACT_DIR/$SERVER_ARCHIVE" sim7600d

echo "Building standalone web artifact"
if [[ ! -d web/client/node_modules ]]; then
  "$NPM" --prefix web/client ci --no-audit --no-fund
fi
if [[ ! -d web/ui/node_modules ]]; then
  "$NPM" --prefix web/ui ci --no-audit --no-fund
fi
"$NPM" --prefix web/ui run build
mkdir -p "$WORK_DIR/web/sim7600d-web"
cp -R web/ui/dist/. "$WORK_DIR/web/sim7600d-web/"
tar -C "$WORK_DIR/web" -czf "$ARTIFACT_DIR/$WEB_ARCHIVE" sim7600d-web

echo "Building server with embedded web UI artifact"
mkdir -p "$WORK_DIR/server-web"
CGO_ENABLED=0 GOOS="$TARGET_GOOS" GOARCH="$TARGET_GOARCH" "$GO" build \
  -trimpath -ldflags "-s -w -X main.Version=$VERSION" \
  -o "$WORK_DIR/server-web/sim7600d" ./cmd/sim7600d
tar -C "$WORK_DIR/server-web" -czf "$ARTIFACT_DIR/$BUNDLE_ARCHIVE" sim7600d

ANDROID_TASK=assembleDebug
ANDROID_APK_SOURCE=android/app/build/outputs/apk/debug/app-debug.apk
if [[ "$ANDROID_SIGNING_READY" == "1" ]]; then
  ANDROID_KEYSTORE_PATH="$WORK_DIR/release.keystore"
  export ANDROID_KEYSTORE_PATH ANDROID_KEYSTORE_PASSWORD ANDROID_KEY_ALIAS ANDROID_KEY_PASSWORD
  printf '%s' "$ANDROID_KEYSTORE_BASE64" | openssl base64 -d -A -out "$ANDROID_KEYSTORE_PATH"
  unset ANDROID_KEYSTORE_BASE64
  chmod 600 "$ANDROID_KEYSTORE_PATH"
  ANDROID_TASK=assembleRelease
  ANDROID_APK_SOURCE=android/app/build/outputs/apk/release/app-release.apk
fi

echo "Building Android APK with $ANDROID_TASK"
(
  cd android
  ./gradlew --no-daemon "$ANDROID_TASK"
)
cp "$ANDROID_APK_SOURCE" "$ARTIFACT_DIR/$ANDROID_APK"

(
  cd "$ARTIFACT_DIR"
  files=("$SERVER_ARCHIVE" "$WEB_ARCHIVE" "$BUNDLE_ARCHIVE" "$ANDROID_APK")
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${files[@]}" > "$CHECKSUMS"
    for file in "${files[@]}"; do
      sha256sum "$file" > "$file.sha256"
    done
  else
    shasum -a 256 "${files[@]}" > "$CHECKSUMS"
    for file in "${files[@]}"; do
      shasum -a 256 "$file" > "$file.sha256"
    done
  fi
)

echo "Artifacts written to $ARTIFACT_DIR"
printf '  %s\n' "$SERVER_ARCHIVE" "$WEB_ARCHIVE" "$BUNDLE_ARCHIVE" "$ANDROID_APK" "$CHECKSUMS"
