#!/usr/bin/env bash
set -e

APP_NAME="Ollama Lab"
BUNDLE_DIR="$APP_NAME.app"
CONTENTS_DIR="$BUNDLE_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

echo "Building Go binary..."
export PATH=$PATH:/usr/local/go/bin:/opt/homebrew/bin
go build -ldflags="-s -w" -o "$APP_NAME" .

echo "Creating macOS .app bundle structure..."
rm -rf "$BUNDLE_DIR"
mkdir -p "$MACOS_DIR"
mkdir -p "$RESOURCES_DIR"

mv "$APP_NAME" "$MACOS_DIR/ollama-lab"
chmod +x "$MACOS_DIR/ollama-lab"

echo "Writing Info.plist..."
cat << 'EOF' > "$CONTENTS_DIR/Info.plist"
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>ollama-lab</string>
    <key>CFBundleIdentifier</key>
    <string>com.local.ollama-lab</string>
    <key>CFBundleName</key>
    <string>Ollama Lab</string>
    <key>CFBundleDisplayName</key>
    <string>Ollama Lab</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSMinimumSystemVersion</key>
    <string>12.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSSupportsAutomaticGraphicsSwitching</key>
    <true/>
</dict>
</plist>
EOF

echo "App bundle created successfully at: $BUNDLE_DIR"
echo "You can launch it with: open \"$BUNDLE_DIR\""
