//go:build !windows

package templates

import "github.com/fsnotify/fsnotify"

func recursiveEvents(string) (<-chan fsnotify.Event, <-chan error, func(), error) {
	return nil, nil, func() {}, nil
}
