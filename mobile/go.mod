module brundlesync/mobile

go 1.26.0

require brundlesync v0.0.0

require (
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	golang.org/x/mobile v0.0.0-20261011034528-197770444867 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

replace brundlesync => ..

tool (
	golang.org/x/mobile/cmd/gobind
	golang.org/x/mobile/cmd/gomobile
)
