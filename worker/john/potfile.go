package john

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (c *Cmd) WatchPotfile() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	c.KillChan = make(chan bool)
	go func() {
		for {
			select {
			case <-c.KillChan:
				c.Log.Info("got kill signal, stop watching potfile")
				watcher.Close()
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				c.Log.Debugf("got %v", event)
				if event.Op&fsnotify.Write == fsnotify.Write {
					c.Results <- c.ReadPotfile()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				c.Log.Error(err)
			}
		}
	}()

	for {
		if err := watcher.Add(potFile); err == nil {
			c.Log.Info("found potfile")
			break
		}
		c.Log.Infof("waiting for file to be created...")
		time.Sleep(time.Second)
	}
}

func (c *Cmd) ReadPotfile() []string {
	b, err := os.ReadFile(potFile)
	if err != nil {
		c.Log.Error(err)
	}

	s := strings.TrimSuffix(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
