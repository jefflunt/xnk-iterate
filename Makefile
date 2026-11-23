APP_NAME := xnk-iterate
MAC_APP := dist/XNK Iterate.app

.PHONY: all build test clean mac-app run

all: test build

test:
	go test -v ./...

build:
	mkdir -p bin
	go build -o bin/$(APP_NAME) ./cmd/xnk-iterate
	go build -o bin/xnk-mcp ./cmd/xnk-mcp

run: build
	./bin/$(APP_NAME)

# Packages a native macOS .app bundle with proper Screen Recording, Microphone, and Speech Info.plist descriptions
mac-app: build
	mkdir -p "$(MAC_APP)/Contents/MacOS" "$(MAC_APP)/Contents/Resources"
	cp bin/$(APP_NAME) "$(MAC_APP)/Contents/MacOS/$(APP_NAME)"
	cp bin/xnk-mcp "$(MAC_APP)/Contents/MacOS/xnk-mcp"
	@if [ -f assets/AppIcon.icns ]; then cp assets/AppIcon.icns "$(MAC_APP)/Contents/Resources/AppIcon.icns"; fi
	printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0">\n<dict>\n  <key>CFBundleName</key><string>XNK Iterate</string>\n  <key>CFBundleDisplayName</key><string>XNK Iterate</string>\n  <key>CFBundleIdentifier</key><string>com.xnk.iterate</string>\n  <key>CFBundleIconFile</key><string>AppIcon</string>\n  <key>CFBundleVersion</key><string>2.3.0</string>\n  <key>CFBundleShortVersionString</key><string>2.3.0</string>\n  <key>CFBundleExecutable</key><string>xnk-iterate</string>\n  <key>CFBundlePackageType</key><string>APPL</string>\n  <key>LSMinimumSystemVersion</key><string>14.0</string>\n  <key>NSHighResolutionCapable</key><true/>\n  <key>NSMicrophoneUsageDescription</key><string>XNK Iterate records live voice commentary and enables two-way Antigravity voice mode.</string>\n  <key>NSSpeechRecognitionUsageDescription</key><string>XNK Iterate transcribes your director commentary locally in real time.</string>\n  <key>NSScreenCaptureUsageDescription</key><string>XNK Iterate records selected windows or displays to extract Trigger and Reaction frames for Google Antigravity.</string>\n</dict>\n</plist>\n' > "$(MAC_APP)/Contents/Info.plist"
	touch "$(MAC_APP)"
	@codesign --force --deep --sign - "$(MAC_APP)" 2>/dev/null || true
	@echo "Built macOS app bundle: $(MAC_APP)"

clean:
	rm -rf bin dist
