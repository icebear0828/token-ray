package agyscanner

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

var portPattern = regexp.MustCompile(`\s(\d+)\s+server\.go:\d+\]\s+Language server listening on random port at (\d+) for HTTP$`)

type ActivePort struct {
	PID  int
	Port int
}

func discoverPorts(logDir string) []ActivePort {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}

	type logFile struct {
		path  string
		mtime time.Time
	}
	var logs []logFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isCliLog(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		logs = append(logs, logFile{filepath.Join(logDir, name), info.ModTime()})
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].mtime.After(logs[j].mtime) })

	seen := make(map[int]bool)
	var ports []ActivePort
	for _, lf := range logs {
		for _, ap := range parseLogPorts(lf.path) {
			if seen[ap.Port] {
				continue
			}
			seen[ap.Port] = true
			if isPortAlive(ap.Port) {
				ports = append(ports, ap)
			}
		}
	}
	return ports
}

func parseLogPorts(path string) []ActivePort {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var ports []ActivePort
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := portPattern.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		pid, _ := strconv.Atoi(m[1])
		port, _ := strconv.Atoi(m[2])
		if port > 0 {
			ports = append(ports, ActivePort{PID: pid, Port: port})
		}
	}
	return ports
}

func isPortAlive(port int) bool {
	conn, err := net.DialTimeout("tcp", "localhost:"+strconv.Itoa(port), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func isCliLog(name string) bool {
	return len(name) > 4 && name[:4] == "cli-" && name[len(name)-4:] == ".log"
}
