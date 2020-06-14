package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const (
	minikubeBin = "/usr/local/bin/minikube"
)

func minikubeStatus() (res []string, running bool) {
	b, cmdErr := exec.Command(minikubeBin, "status", "--output=json").CombinedOutput()
	var m map[string]interface{}
	unErr := json.Unmarshal(b, &m)
	if unErr != nil {
		// TODO inspect cmdErr
		_ = cmdErr
		return
	}

	status := strings.ToLower(fmt.Sprint(m["Host"]))
	res = append(res, "📦 minikube "+status)
	running = status != "stopped"

	return
}

func minikubeStop() {
	_ = exec.Command(minikubeBin, "stop").Run()
}

func minikubeDelete() {
	_ = exec.Command(minikubeBin, "delete").Run()
}
