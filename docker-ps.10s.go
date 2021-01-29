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
	"context"
	"encoding/hex"
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

	"golang.org/x/sync/errgroup"
)

const dockerBin = "/usr/local/bin/docker"

type containerType int

const (
	Single containerType = iota
	Group
	Compose
	Kubernetes
	Minikube
	Talos
)

type project struct {
	typ  containerType
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
	State        string `json:"State"`
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
		case "com.github.AlekSi.docker-ps.group":
			c.project.typ = Group
			c.project.name = v
		case "com.docker.compose.project":
			c.project.typ = Compose
			c.project.name = v
		case "io.kubernetes.pod.namespace":
			c.project.typ = Kubernetes
			c.project.name = v
			c.Image = "" // remove very long image name with sha256 hash tag
		case "name.minikube.sigs.k8s.io":
			c.project.typ = Minikube
			c.project.name = v
			c.Image = "" // remove very long image name with sha256 hash tag
		case "talos.cluster.name":
			c.project.typ = Talos
			c.project.name = v
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
	return c.State == "running" || strings.HasPrefix(c.Status, "Up ")
}

// containerLs returns all containers sorted by "project" (Docker Compose project, Kubernetes namespace,
// Minikube profile name, Talos cluster) and name.
func containerLs() ([]container, error) {
	cmd := exec.Command(dockerBin, "container", "ls", "--all", "--no-trunc", "--format={{json .}}")
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

// network contains parsed `docker network ls` output for a single network.
type network struct {
	CreatedAt string `json:"CreatedAt"`
	Driver    string `json:"Driver"`
	ID        string `json:"ID"`
	Name      string `json:"Name"`
	Scope     string `json:"Scope"`
}

// networkLs returns all networks.
func networkLs() ([]network, error) {
	cmd := exec.Command(dockerBin, "network", "ls", "--no-trunc", "--format={{json .}}")
	cmd.Stderr = os.Stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var networks []network
	d := json.NewDecoder(bytes.NewReader(b))
	for {
		var n network
		if err = d.Decode(&n); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		networks = append(networks, n)
	}

	sort.Slice(networks, func(i int, j int) bool {
		if networks[i].Driver != networks[j].Driver {
			return networks[i].Driver < networks[j].Driver
		}
		return networks[i].Name < networks[j].Name
	})

	return networks, nil
}

// volume contains parsed `docker volume ls` output for a single volume.
type volume struct {
	Driver string `json:"Driver"`
	Name   string `json:"Name"`
}

func (v *volume) anonymous() bool {
	if v.Driver != "local" {
		return false
	}

	if len(v.Name) != 64 {
		return false
	}
	_, err := hex.DecodeString(v.Name)
	return err == nil
}

// volumeLs returns all volumes.
func volumeLs() ([]volume, error) {
	cmd := exec.Command(dockerBin, "volume", "ls", "--format={{json .}}")
	cmd.Stderr = os.Stderr
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var volumes []volume
	d := json.NewDecoder(bytes.NewReader(b))
	for {
		var v volume
		if err = d.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		volumes = append(volumes, v)
	}

	sort.Slice(volumes, func(i int, j int) bool {
		if volumes[i].Driver != volumes[j].Driver {
			return volumes[i].Driver < volumes[j].Driver
		}
		return volumes[i].Name < volumes[j].Name
	})

	return volumes, nil
}

func containerCmd(command, projectName string) {
	containers, err := containerLs()
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
		case "start", "rm":
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

	args := []string{command}
	if command == "rm" {
		args = append(args, "--force", "--volumes")
	}
	args = append(args, ids...)
	cmd := exec.Command(dockerBin, args...)
	log.Print(strings.Join(cmd.Args, " "))
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func pruneCmd() {
	for _, cmdline := range []string{
		"buildx prune --force",
		"system prune --force --volumes",
	} {
		cmd := exec.Command(dockerBin, strings.Split(cmdline, " ")...)
		log.Print(strings.Join(cmd.Args, " "))
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Print(err)
		}
	}
}

func defaultCmd(ctx context.Context) {
	bin, _ := os.Executable()

	var containers []container
	var networks []network
	var volumes []volume

	g, ctx := errgroup.WithContext(ctx)
	_ = ctx // TODO
	g.Go(func() error {
		var err error
		containers, err = containerLs()
		return err
	})
	g.Go(func() error {
		var err error
		networks, err = networkLs()
		return err
	})
	g.Go(func() error {
		var err error
		volumes, err = volumeLs()
		return err
	})
	if err := g.Wait(); err != nil {
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
			case Group:
				fmt.Printf("🐳 %s\n", lastProjectName)

			case Compose:
				fmt.Printf("🐙 %s\n", lastProjectName)

				fmt.Printf("-- ▶️ Start all | bash=%q param1=-project=%s param2=start terminal=false refresh=true\n", bin, lastProjectName)
				fmt.Printf("-- 🔄 Restart all | bash=%q param1=-project=%s param2=restart terminal=false refresh=true\n", bin, lastProjectName)
				fmt.Printf("-- ⏹ Stop all | bash=%q param1=-project=%s param2=stop terminal=false refresh=true\n", bin, lastProjectName)
				fmt.Printf("-- ⏬ Stop and remove all | bash=%q param1=-project=%s param2=kill param3=rm terminal=false refresh=true\n", bin, lastProjectName)

			case Kubernetes:
				fmt.Printf("☸️ %s\n", lastProjectName)

			case Minikube:
				fmt.Printf("📦 %s\n", lastProjectName)

			case Talos:
				fmt.Printf("🔺 %s\n", lastProjectName)

				fmt.Printf("-- ▶️ Start all | bash=%q param1=-project=%s param2=start terminal=false refresh=true\n", bin, lastProjectName)
				fmt.Printf("-- ⏹ Stop all | bash=%q param1=-project=%s param2=stop terminal=false refresh=true\n", bin, lastProjectName)
				fmt.Printf("-- 🔄 Restart all | bash=%q param1=-project=%s param2=restart terminal=false refresh=true\n", bin, lastProjectName)
				fmt.Printf("-- ⏬ Stop and remove all | bash=%q param1=-project=%s param2=kill param3=rm terminal=false refresh=true\n", bin, lastProjectName)

			default:
				log.Fatalf("Unexpected project type %v.", c.project.typ)
			}
		}

		icon := "🐳"
		if strings.HasPrefix(c.Image, "moby/buildkit:") {
			icon = "⚙️"
		}

		fmt.Printf("%s %s ", icon, c.Names)
		if c.Image != "" {
			fmt.Printf("(%s) ", c.Image)
		}
		if c.running() {
			fmt.Printf("| color=green bash=%q param1=stop param2=%s terminal=false refresh=true\n", dockerBin, c.ID)
		} else {
			fmt.Printf("| color=red bash=%q param1=start param2=%s terminal=false refresh=true\n", dockerBin, c.ID)
		}
	}

	if len(networks) != 0 {
		fmt.Println("---")
		fmt.Printf("%d networks\n", len(networks))
		for _, n := range networks {
			fmt.Printf("%s (%s)\n", n.Name, n.Driver)
		}
	}

	if len(volumes) != 0 {
		var anonymous int
		fmt.Println("---")
		fmt.Printf("%d volumes\n", len(volumes))
		for _, v := range volumes {
			if v.anonymous() {
				anonymous++
				continue
			}
			fmt.Printf("%s (%s)\n", v.Name, v.Driver)
		}

		if anonymous != 0 {
			fmt.Printf("%d anonymous\n", anonymous)
		}
	}

	if bin != "" {
		fmt.Println("---")
		fmt.Printf("⭕️ Stop all containers | bash=%q param1=stop terminal=false refresh=true\n", bin)
		fmt.Printf("🛑 Remove stopped containers | bash=%q param1=rm terminal=false refresh=true\n", bin)
		fmt.Printf("⛔️ Prune orphan data | bash=%q param1=-prune terminal=false refresh=true\n", bin)
		fmt.Printf("📛 Stop, remove and and prune everything | bash=%q param1=-prune param2=kill terminal=false refresh=true\n", bin)
	}
}

func main() {
	projectF := flag.String("project", "", `"project" (Docker Compose project, Kubernetes namespace, Minikube profile name, Talos cluster)`)
	pruneF := flag.Bool("prune", false, `prune stopped containers, networks, volumes, and caches`)
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags] [command]\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Commands: start, stop, restart, rm, kill.\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() == 0 {
		defaultCmd(context.TODO())
	} else {
		for _, c := range flag.Args() {
			containerCmd(c, *projectF)
		}
	}

	if *pruneF {
		pruneCmd()
	}
}
