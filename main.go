package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hpcloud/tail"
)

var tailers = make(map[string]bool)

type event struct {
	path string
	line *tail.Line
}

func main() {
	config := tail.Config{}

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [file|pattern ...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.BoolVar(&config.ReOpen, "F", false, "Reopen recreated files")
	flag.BoolVar(&config.Follow, "f", false, "Continue looking for new lines ")
	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
	}

	events := make(chan event)
	waitGroup := sync.WaitGroup{}

	paths := os.Args[1:]
	createAllTailers(paths, config, events, &waitGroup)

	go func() {
		for {
			time.Sleep(time.Second)
			createAllTailers(paths, config, events, &waitGroup)
		}
	}()

	go func() {
		lastPath := ""
		for e := range events {
			if lastPath != e.path {
				log.Println("Found new content in", e.path)
				lastPath = e.path
			}
			if e.line.Err != nil {
				log.Println("Error tailing", e.path, ":", e.line.Err)
			}
			fmt.Println(e.line.Text)
		}
	}()

	waitGroup.Wait()
}

func createAllTailers(paths []string, config tail.Config, events chan<- event, waitGroup *sync.WaitGroup) {
	for _, path := range paths {
		createTailers(path, config, events, waitGroup)
	}
}

func createTailers(pattern string, config tail.Config, events chan<- event, waitGroup *sync.WaitGroup) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			log.Println(match, "stat error:", err)
			continue
		}

		if info.IsDir() {
			continue
		}

		if _, ok := tailers[match]; ok {
			continue
		}

		config.Location = &tail.SeekInfo{Offset: -500, Whence: 2}
		if info.Size() < 500 {
			config.Location.Offset = -info.Size()
		}

		t, err := tail.TailFile(match, config)
		if err != nil {
			log.Println("Error tailing", match, err)
			continue
		}

		tailers[match] = true
		waitGroup.Add(1)
		go func(path string) {
			defer waitGroup.Done()
			if config.Location.Offset == -500 {
				<-t.Lines // throw away one line
			}
			for line := range t.Lines {
				events <- event{path, line}
			}
		}(match)
	}

	return nil
}
