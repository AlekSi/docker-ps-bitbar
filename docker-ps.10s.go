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
	"sort"
	"strings"
	"time"
)

const dockerBin = "/usr/local/bin/docker"

type ContainerType int

const (
	Single ContainerType = iota
	Compose
	Kubernetes
)

type project struct {
	typ  ContainerType
	name string
}

// container contains parsed `docker ps` output for a single container.
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

	project project
}

// fill sets project field, and may also change other fields.
func (c *container) fill() {
	c.project.typ = Single

	for _, part := range strings.Split(c.Labels, ",") {
		pair := strings.Split(part, "=")
		if len(pair) != 2 {
			continue
		}

		k, v := pair[0], pair[1]
		switch k {
		case "com.docker.compose.project":
			c.project.typ = Compose
			c.project.name = v
		case "io.kubernetes.pod.namespace":
			c.project.typ = Kubernetes
			c.project.name = v

			// remove very long image name with sha256 hash tag
			c.Image = ""
		}

		if c.project.name != "" {
			return
		}
	}
}

func (c *container) createdAt() time.Time {
	t, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", c.CreatedAt)
	return t
}

func (c *container) running() bool {
	return strings.HasPrefix(c.Status, "Up ")
}

// ps returns all containers sorted by "project" (Docker Compose project, Kubernetes namespace) and name.
func ps() ([]container, error) {
	cmd := exec.Command(dockerBin, "ps", "--all", "--no-trunc", "--format={{json .}}")
	cmd.Stderr = os.Stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var containers []container
	d := json.NewDecoder(bytes.NewReader(b))
	for {
		var c container
		if err = d.Decode(&c); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		c.fill()
		containers = append(containers, c)
	}

	sort.Slice(containers, func(i int, j int) bool {
		if containers[i].project.typ != containers[j].project.typ {
			return containers[i].project.typ < containers[j].project.typ
		}
		if containers[i].project.name != containers[j].project.name {
			return containers[i].project.name < containers[j].project.name
		}
		return containers[i].Names < containers[j].Names
	})

	return containers, nil
}

func containerCmd(command, projectName string) {
	containers, err := ps()
	if err != nil {
		log.Fatal(err)
	}

	var ids []string
	for _, c := range containers {
		if projectName != "" && projectName != c.project.name {
			continue
		}

		var add bool
		switch command {
		case "start":
			add = !c.running()
		case "restart":
			add = true
		case "stop", "kill":
			add = c.running()
		default:
			log.Fatalf("Unexpected command %s.", command)
		}

		if add {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return
	}

	args := append([]string{command}, ids...)
	cmd := exec.Command(dockerBin, args...)
	log.Print(strings.Join(cmd.Args, " "))
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func pruneCmd() {
	cmd := exec.Command(dockerBin, "system", "prune", "--force", "--volumes")
	log.Print(strings.Join(cmd.Args, " "))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
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
		var total, running int
		for _, c := range containers {
			total++
			if c.running() {
				running++
			}
		}
		fmt.Printf("🐳%d/%d\n", running, total)
	}
	fmt.Println("---")

	var lastProjectName string
	for _, c := range containers {
		if lastProjectName != c.project.name {
			lastProjectName = c.project.name

			fmt.Println("---")
			switch c.project.typ {
			case Compose:
				fmt.Printf("🐙 %s\n", lastProjectName)

				fmt.Printf("-- ▶️ Start all | bash=%q param1=-project=%s param2=start terminal=false refresh=true\n", bin, lastProjectName)
				fmt.Printf("-- ⏹ Stop all | bash=%q param1=-project=%s param2=stop terminal=false refresh=true\n", bin, lastProjectName)
				fmt.Printf("-- 🔄 Restart all | bash=%q param1=-project=%s param2=restart terminal=false refresh=true\n", bin, lastProjectName)

			case Kubernetes:
				fmt.Printf("☸️ %s\n", lastProjectName)

			default:
				log.Fatalf("Unexpected project type %v.", c.project.typ)
			}
		}

		fmt.Printf("%s ", c.Names)
		if c.Image != "" {
			fmt.Printf("(%s) ", c.Image)
		}
		if c.running() {
			fmt.Printf("| color=green bash=%q param1=stop param2=%s terminal=false refresh=true\n", dockerBin, c.ID)
		} else {
			fmt.Printf("| color=red bash=%q param1=start param2=%s terminal=false refresh=true\n", dockerBin, c.ID)
		}
	}

	if bin != "" {
		fmt.Println("---")
		fmt.Printf("🛑 Stop all | bash=%q param1=stop terminal=false refresh=true\n", bin)
		fmt.Printf("📛 Prune | bash=%q param1=-prune terminal=false refresh=true\n", bin)
		fmt.Printf("🧨 Kill all and prune | bash=%q param1=-prune param2=kill terminal=false refresh=true\n", bin)
	}
}

func main() {
	projectF := flag.String("project", "", `"project" (Docker Compose project, Kubernetes namespace)`)
	pruneF := flag.Bool("prune", false, `prune all data`)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] [command]\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Commands: start, stop, restart, kill.\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	switch flag.NArg() {
	case 0:
		defaultCmd()
	case 1:
		containerCmd(flag.Arg(0), *projectF)
	default:
		flag.Usage()
		os.Exit(2)
	}

	if *pruneF {
		pruneCmd()
	}
}
