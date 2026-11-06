package main

import "runtime"

func targetDescription() string {
	return "target: " + runtime.GOOS + "/" + runtime.GOARCH
}
