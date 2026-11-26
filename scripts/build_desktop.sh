#!/bin/bash

set -e

echo "🖥️  Building AI Flight Dashboard Desktop App..."

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

go build -trimpath -ldflags="-s -w" -tags desktop,production -o build/bin/ai-flight-dashboard ./cmd/dashboard

echo ""
echo "✅ Desktop binary built at: build/bin/ai-flight-dashboard"
echo ""
echo "Usage:"
echo "  ./build/bin/ai-flight-dashboard          # Native desktop window"
echo "  ./build/bin/ai-flight-dashboard --web    # Web dashboard mode"
echo "  ./build/bin/ai-flight-dashboard --tui    # TUI terminal mode"

# Platform-specific packaging
if [[ "$(uname)" == "Darwin" ]]; then
    echo "📦 Packaging macOS .app bundle..."
    # 1. Run wails build to generate the .app structure (icons, plist, etc.)
    # This generates a dummy archive executable because main.go is not in the root directory
    wails build -platform darwin/arm64 -skipbindings > /dev/null 2>&1 || true
    
    # 2. Overwrite the dummy executable with the correctly compiled binary
    cp build/bin/ai-flight-dashboard "build/bin/AI Flight Dashboard.app/Contents/MacOS/ai-flight-dashboard"
    
    # 3. Resign the app bundle to prevent macOS Gatekeeper from rejecting it
    codesign --force --deep --sign - "build/bin/AI Flight Dashboard.app"
    
    echo "✅ macOS .app bundle created at: build/bin/AI Flight Dashboard.app"
    echo "   You can now double-click it to run the application!"
fi
