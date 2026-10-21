GO ?= go
GUI_GO ?= $(GO)
PREFIX ?= $(HOME)/.local
GUI_BINARY = BrundleSyncGUI
GUI_LDFLAGS ?=
ifeq ($(OS),Windows_NT)
GUI_BINARY = BrundleSyncGUI.exe
GUI_LDFLAGS = -H=windowsgui
endif

.PHONY: build gui install-gui android test test-interfaces check clean
build:
	CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -o BrundleSync .

gui:
	cd desktop && CGO_ENABLED=1 $(GUI_GO) build -buildvcs=false -trimpath -ldflags "$(GUI_LDFLAGS)" -o ../$(GUI_BINARY) .

install-gui: gui
	install -Dm755 BrundleSyncGUI $(PREFIX)/bin/BrundleSyncGUI
	install -Dm644 desktop/packaging/ca.cerberusgames.douchesync.desktop $(PREFIX)/share/applications/ca.cerberusgames.douchesync.desktop
	install -Dm644 desktop/assets/brundlesync.svg $(PREFIX)/share/icons/hicolor/scalable/apps/ca.cerberusgames.douchesync.svg

android:
	cd android && GO="$(GO)" ./gradlew :app:assembleDebug

test-interfaces:
	cd phoneui && $(GO) test -tags ci ./...
	cd desktop && $(GO) test -tags ci ./...
	cd mobile && $(GO) test ./...

test:
	$(GO) test -buildvcs=false ./...

check:
	$(GO) test -race -buildvcs=false ./...
	$(GO) vet -buildvcs=false ./...

clean:
	rm -f BrundleSync BrundleSync.exe BrundleSyncGUI BrundleSyncGUI.exe
