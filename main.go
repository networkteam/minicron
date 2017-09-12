// Minicron
//
// A simplified cron scheduler for running multiple scheduled commands.
//
// Example:
//
//     minicron -c "catchup" "@every 10s" "./flow event:catchup --verbose"\
//              -c "sync" "0 0 7 * * *" "./flow content:sync"
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/kvz/logstreamer"
	"github.com/mgutz/ansi"
	"github.com/robfig/cron"
)

var (
	verbose       = false
	jobIDSequence = 1
)

func main() {
	c := cron.New()
	addJobs(c, os.Args)
	c.Run()
}

var (
	ansiGreen  = ansi.ColorFunc("green+b")
	ansiYellow = ansi.ColorFunc("yellow+b")
	ansiWhite  = ansi.ColorFunc("white+b")
	ansiRed    = ansi.ColorFunc("red+b")
)

func addJobs(c *cron.Cron, args []string) {
	argn := len(args)
	for i, arg := range args {
		if arg == "-v" {
			verbose = true
		} else if arg == "-c" {
			if i+3 >= argn {
				fmt.Println("missing arguments for command, expected \"-c [name] [schedule] [command]\"")
				os.Exit(1)
			}
			name, schedule, command := args[i+1], args[i+2], args[i+3]
			j := newCmdJob(name, schedule, command)
			c.AddJob(schedule, j)
		}
	}
}

type cmdJob struct {
	id          int
	name        string
	cmd         string
	invocations int
	tryLockSem  chan bool
	logger      *log.Logger
}

func (c *cmdJob) Run() {
	select {
	// Try to get the lock
	case v := <-c.tryLockSem:
		// Put the lock back in any case after finishing or error / panic
		defer func() { c.tryLockSem <- v }()

		if verbose {
			c.log("Running", ansiWhite(c.cmd))
		}
		cmdAndArgs := strings.Split(c.cmd, " ")
		p := exec.Command(cmdAndArgs[0], cmdAndArgs[1:]...)

		logStreamerOut := logstreamer.NewLogstreamer(c.logger, "stdout", false)
		defer logStreamerOut.Close()
		logStreamerErr := logstreamer.NewLogstreamer(c.logger, "stderr", false)
		defer logStreamerErr.Close()

		p.Stdout = logStreamerOut
		p.Stderr = logStreamerErr

		if err := p.Run(); err != nil {
			c.log(ansiRed("Error: " + err.Error()))
		}
	default:
		c.log("Skipped because still running")
	}
}

func (c cmdJob) log(args ...interface{}) {
	c.logger.Println(args...)
}

func newCmdJob(name, schedule, cmd string) cron.Job {
	id := jobIDSequence
	jobIDSequence++

	tryLockSem := make(chan bool, 1)
	tryLockSem <- true

	prefix := ansi.Color("["+name+"] ", fmt.Sprintf("%d+h", (id*31)%255))
	logger := log.New(os.Stdout, prefix, 0)

	j := &cmdJob{id: id, name: name, cmd: cmd, tryLockSem: tryLockSem, logger: logger}
	if verbose {
		j.log("Scheduling", ansiWhite(cmd), ansiGreen(schedule))
	}
	return j
}
