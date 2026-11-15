#!/bin/bash

set -e

echo "🖥️  Building Token Ray Desktop App..."

# 1. Build frontend
echo "📦 Building frontend..."
cd frontend
npm install --silent
npm run build
cd ..

# 2. Build desktop binary with correct Wails build tags
echo "🔨 Building desktop binary..."

# macOS requires the UniformTypeIdentifiers framework for Wails v2
if [[ "$(uname)" == "Darwin" ]]; then
    export CGO_LDFLAGS="-framework UniformTypeIdentifiers"
fi

go build -tags desktop,production -ldflags "-w -s" -o build/bin/token-ray ./cmd/dashboard

echo ""
echo "✅ Desktop binary built at: build/bin/token-ray"
echo ""
echo "Usage:"
echo "  ./build/bin/token-ray          # Native desktop window"
echo "  ./build/bin/token-ray --web    # Web dashboard mode"
echo "  ./build/bin/token-ray --tui    # TUI terminal mode"

# Platform-specific packaging
if [[ "$(uname)" == "Darwin" ]]; then
    echo "📦 Packaging macOS .app bundle..."
    # 1. Run wails build to generate the .app structure (icons, plist, etc.)
    # This generates a dummy archive executable because main.go is not in the root directory
    wails build -platform darwin/arm64 -skipbindings > /dev/null 2>&1 || true
    
    # 2. Overwrite the dummy executable with the correctly compiled binary
    cp build/bin/token-ray "build/bin/Token Ray.app/Contents/MacOS/token-ray"
    
    # 3. Resign the app bundle to prevent macOS Gatekeeper from rejecting it
    codesign --force --deep --sign - "build/bin/Token Ray.app"
    
    echo "✅ macOS .app bundle created at: build/bin/Token Ray.app"
    echo "   You can now double-click it to run the application!"
fi
