//usr/bin/env go run $0 $@; exit $?

// <bitbar.title>docker-ps</bitbar.title>
// <bitbar.version>v1.0</bitbar.version>
// <bitbar.author>Alexey Palazhchenko</bitbar.author>
// <bitbar.author.github>AlekSi</bitbar.author.github>
// <bitbar.desc>Displays statuses of local Docker containers.</bitbar.desc>
// <bitbar.dependencies>go,docker</bitbar.dependencies>
// <bitbar.abouturl>https://github.com/AlekSi/docker-ps-bitbar</bitbar.abouturl>

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

const dockerBin = "/usr/local/bin/docker"

type container struct {
	Command      string `json:"Command"`
	CreatedAt    string `json:"CreatedAt"`
	ID           string `json:"ID"`
	Image        string `json:"Image"`
	Labels       string `json:"Labels"`
	LocalVolumes string `json:"LocalVolumes"`
	Mounts       string `json:"Mounts"`
	Names        string `json:"Names"`
	Networks     string `json:"Networks"`
	Ports        string `json:"Ports"`
	RunningFor   string `json:"RunningFor"`
	Size         string `json:"Size"`
	Status       string `json:"Status"`
}

func (c *container) createdAt() time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", c.CreatedAt)
	return t
}

func (c *container) running() bool {
	return strings.HasPrefix(c.Status, "Up ")
}

func ps() (res []container, err error) {
	cmd := exec.Command(dockerBin, "ps", "--all", "--no-trunc", "--format={{json .}}")
	var b []byte
	if b, err = cmd.Output(); err != nil {
		return
	}

	d := json.NewDecoder(bytes.NewReader(b))
	for {
		var c container
		if err = d.Decode(&c); err != nil {
			if err == io.EOF {
				err = nil
			}
			return
		}
		res = append(res, c)
	}
}

func pruneCmd() {
	cmd := exec.Command(dockerBin, "system", "prune", "--force", "--volumes")
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func wipeCmd() {
	containers, err := ps()
	if err != nil {
		log.Fatal(err)
	}

	var ids []string
	for _, c := range containers {
		if c.running() {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) > 0 {
		args := append([]string{"kill"}, ids...)
		cmd := exec.Command(dockerBin, args...)
		if err = cmd.Run(); err != nil {
			log.Fatal(err)
		}
	}

	pruneCmd()
}

func defaultCmd() {
	bin, _ := os.Executable()

	containers, err := ps()
	if err != nil {
		log.Fatal(err)
	}

	if len(containers) == 0 {
		fmt.Println("🐳")
	} else {
		var running int
		for _, c := range containers {
			if c.running() {
				running++
			}
		}
		fmt.Printf("🐳%d/%d\n", running, len(containers))
	}
	fmt.Println("---")

	for _, c := range containers {
		fmt.Printf("%s (%s) | ", c.Names, c.Image)
		if c.running() {
			fmt.Printf("color=green bash=%q param1=stop param2=%s terminal=false refresh=true\n", dockerBin, c.ID)
		} else {
			fmt.Printf("color=red bash=%q param1=start param2=%s terminal=false refresh=true\n", dockerBin, c.ID)
		}
	}
	fmt.Println("---")

	fmt.Printf("🛑 Stop all | bash=%q param1=stop ", dockerBin)
	for i, c := range containers {
		fmt.Printf("param%d=%s ", i+2, c.ID)
	}
	fmt.Println("terminal=false refresh=true")

	if bin != "" {
		fmt.Printf("📛 Prune | bash=%q param1=-prune terminal=false refresh=true\n", bin)
		fmt.Printf("🧨 Stop all and prune | bash=%q param1=-wipe terminal=false refresh=true\n", bin)
	}
}

func main() {
	pruneF := flag.Bool("prune", false, "prune all data")
	wipeF := flag.Bool("wipe", false, "stop all containers and prune all data")
	flag.Parse()

	switch {
	case *pruneF:
		pruneCmd()
	case *wipeF:
		wipeCmd()
	default:
		defaultCmd()
	}
}
