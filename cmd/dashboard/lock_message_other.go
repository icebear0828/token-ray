//go:build !windows

package main

import "log"

func showAlreadyRunningMessage(dataDir string) {
	log.Printf("AI Flight Dashboard is already running with data dir %s. Stop it before starting a second instance.", dataDir)
}
