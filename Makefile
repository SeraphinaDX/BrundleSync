GO ?= go
GUI_GO ?= $(GO)
PREFIX ?= $(HOME)/.local
GUI_BINARY = DoucheSyncGUI
GUI_LDFLAGS ?=
ifeq ($(OS),Windows_NT)
GUI_BINARY = DoucheSyncGUI.exe
GUI_LDFLAGS = -H=windowsgui
endif

.PHONY: build gui install-gui android test test-interfaces check clean
build:
	CGO_ENABLED=0 $(GO) build -buildvcs=false -trimpath -o DoucheSync .

gui:
	cd desktop && CGO_ENABLED=1 $(GUI_GO) build -buildvcs=false -trimpath -ldflags "$(GUI_LDFLAGS)" -o ../$(GUI_BINARY) .

install-gui: gui
	install -Dm755 DoucheSyncGUI $(PREFIX)/bin/DoucheSyncGUI
	install -Dm644 desktop/packaging/ca.cerberusgames.douchesync.desktop $(PREFIX)/share/applications/ca.cerberusgames.douchesync.desktop
	install -Dm644 desktop/assets/douchesync.svg $(PREFIX)/share/icons/hicolor/scalable/apps/ca.cerberusgames.douchesync.svg

android:
	cd android && GO="$(GO)" ./gradlew :app:assembleDebug

test-interfaces:
	cd desktop && $(GO) test -tags ci ./...
	cd mobile && $(GO) test ./...

test:
	$(GO) test -buildvcs=false ./...

check:
	$(GO) test -race -buildvcs=false ./...
	$(GO) vet -buildvcs=false ./...

clean:
	rm -f DoucheSync DoucheSync.exe DoucheSyncGUI DoucheSyncGUI.exe
