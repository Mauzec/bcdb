package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	basePort    = 8000
	serverBase  = "http://127.0.0.1"
	key         = "foo"
	runs        = 3
	reqs        = 500
	conc        = 25
	ordinaryCnt = 10
	targetH     = 10000
	tplDir      = "data_template"
)

func main() {

	var totalList []int
	for i := 12; i <= 100; i += 2 {
		totalList = append(totalList, i)
	}

	vMax := totalList[len(totalList)-1] - ordinaryCnt
	if _, err := os.Stat(tplDir); os.IsNotExist(err) {
		fmt.Printf("❯ Pre-generate template cluster: validators=%d ordinary=%d up to H=%d\n",
			vMax, ordinaryCnt, targetH)
		if err := preGenerateTemplate(vMax, ordinaryCnt); err != nil {
			log.Fatalf("template gen failed: %v", err)
		}
	}

	for _, total := range totalList {
		v := total - ordinaryCnt
		if v < 1 {
			fmt.Printf("skip total=%d: need ≥1 validator\n\n", total)
			continue
		}
		o := total - v
		fmt.Printf("=== total=%d → v=%d, o=%d ===\n", total, v, o)

		os.RemoveAll("data")

		for i := 1; i <= v; i++ {
			cpDir(filepath.Join(tplDir, fmt.Sprintf("validator%d", i)),
				filepath.Join("data", fmt.Sprintf("validator%d", i)))
		}
		for i := 1; i <= o; i++ {
			cpDir(filepath.Join(tplDir, fmt.Sprintf("node%d", i)),
				filepath.Join("data", fmt.Sprintf("node%d", i)))
		}

		pids := startCluster(v, o)

		time.Sleep(2 * time.Second)

		server := serverBase + ":" + strconv.Itoa(basePort)
		out, _ := exec.Command("go", "run", "cmd/test_s/bench_query.go",
			"-server="+server,
			"-key="+key,
			"-runs="+strconv.Itoa(runs),
			"-n="+strconv.Itoa(reqs),
			"-c="+strconv.Itoa(conc),
		).CombinedOutput()

		last := lastLine(string(out))
		fmt.Printf("→ total=%d, v=%d: %s\n\n", total, v, last)

		for _, pid := range pids {
			pid.Process.Kill()
			pid.Process.Wait()
		}
	}
}

func preGenerateTemplate(vMax, oCnt int) error {

	pids := startCluster(vMax, oCnt, tplDir)

	time.Sleep(2 * time.Second)

	client := &http.Client{Timeout: 5 * time.Second}
	urlBase := fmt.Sprintf("%s:%d", serverBase, basePort)
	for {

		resp, err := client.Get(urlBase + "/chain")
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var chain []interface{}
		json.NewDecoder(resp.Body).Decode(&chain)
		resp.Body.Close()
		if len(chain)%1000 == 0 {
			fmt.Println(len(chain))
		}
		if len(chain) >= targetH {
			break
		}

		blkURL := fmt.Sprintf("%s/addblock?key=k%d&value=v%d",
			urlBase, len(chain), len(chain))
		client.Get(blkURL)
	}

	for _, cmd := range pids {
		cmd.Process.Kill()
		cmd.Process.Wait()
	}
	return nil
}

func startCluster(v, o int, baseDir ...string) []*exec.Cmd {
	dirRoot := "data"
	if len(baseDir) > 0 {
		dirRoot = baseDir[0]
	}
	var cmds []*exec.Cmd
	port := basePort

	spawn := func(id string) {
		dir := filepath.Join(dirRoot, id)
		os.MkdirAll(dir, 0o755)
		cmd := exec.Command("go", "run", "cmd/test/main.go",
			"--id="+id, "--port="+strconv.Itoa(port))
		cmd.Env = append(os.Environ(),
			"ADS_PATH="+filepath.Join(dir, "ads.db"),
			"BLK_PATH="+filepath.Join(dir, "blockchain.db"),
		)
		logf, _ := os.Create("/tmp/" + id + ".log")
		cmd.Stdout = logf
		cmd.Stderr = logf
		if err := cmd.Start(); err != nil {
			log.Fatalf("start %s: %v", id, err)
		}
		cmds = append(cmds, cmd)
		port++
	}

	for i := 1; i <= v; i++ {
		spawn(fmt.Sprintf("validator%d", i))
	}
	for i := 1; i <= o; i++ {
		spawn(fmt.Sprintf("node%d", i))
	}
	return cmds
}

func cpDir(src, dst string) {
	os.MkdirAll(dst, 0o755)
	exec.Command("cp", "-r", src+string(os.PathSeparator)+".", dst).Run()
}

func lastLine(out string) string {
	var last string
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		t := s.Text()
		if strings.TrimSpace(t) != "" {
			last = t
		}
	}
	return last
}
