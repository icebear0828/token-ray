//go:build !windows

package main

import "log"

func showAlreadyRunningMessage(dataDir string) {
	log.Printf("Token Ray is already running with data dir %s. Stop it before starting a second instance.", dataDir)
}
