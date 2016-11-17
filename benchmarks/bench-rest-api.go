// This file is part of Graylog.
//
// Graylog is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Graylog is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with Graylog.  If not, see <http://www.gnu.org/licenses/>.

// This runs heartbeat and configuration requests against the server API
//
// Useful for profiling/benchmarking the sidecar <-> server communication without running an actual sidecar process.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/Graylog2/collector-sidecar/api"
	"github.com/Graylog2/collector-sidecar/api/rest"
	"github.com/Graylog2/collector-sidecar/cfgfile"
	"github.com/Graylog2/collector-sidecar/common"
	"github.com/Graylog2/collector-sidecar/context"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var flagNum int
var flagInterval int
var flagUrl string
var flagStatus bool

func init() {
	flag.IntVar(&flagNum, "num", 10, "number of sidecar processes")
	flag.IntVar(&flagInterval, "interval", 1, "update interval")
	flag.StringVar(&flagUrl, "url", "http://127.0.0.1:9000/api/", "graylog server url")
	flag.BoolVar(&flagStatus, "status", true, "send status")
	flag.Parse()
}

var httpClient = rest.NewHTTPClient(&tls.Config{})

func startHeartbeat(ctx *context.Ctx, done chan bool, wg *sync.WaitGroup) {
	fmt.Printf("[%s] starting heartbeat\n", ctx.UserConfig.NodeId)
	for {
		select {
		case <-done:
			fmt.Printf("[%s] stopping heartbeat\n", ctx.UserConfig.NodeId)
			wg.Done()
			return
		default:
			time.Sleep(time.Duration(ctx.UserConfig.UpdateInterval) * time.Second)
			statusRequest := api.NewStatusRequest()
			api.UpdateRegistration(httpClient, ctx, &statusRequest)
		}
	}
}

func startConfigUpdater(ctx *context.Ctx, done chan bool, wg *sync.WaitGroup) {
	fmt.Printf("[%s] starting config updater\n", ctx.UserConfig.NodeId)
	for {
		select {
		case <-done:
			fmt.Printf("[%s] stopping config updater\n", ctx.UserConfig.NodeId)
			wg.Done()
			return
		default:
			time.Sleep(time.Duration(ctx.UserConfig.UpdateInterval) * time.Second)
			_, err := api.RequestConfiguration(httpClient, ctx)
			if err != nil {
				fmt.Printf("[%s] can't fetch config from Graylog API: %v\n", ctx.UserConfig.NodeId, err)
				return
			}
		}
	}
}

func main() {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool)
	var wg sync.WaitGroup

	for i := 1; i <= flagNum; i++ {
		pid := i

		ctx := context.NewContext()
		ctx.ServerUrl, _ = url.Parse(flagUrl)
		ctx.CollectorId = common.RandomUuid()
		ctx.UserConfig = &cfgfile.SidecarConfig{
			NodeId:         fmt.Sprintf("sidecar-benchmark-%03d", pid),
			Tags:           []string{"linux"},
			UpdateInterval: flagInterval,
			SendStatus:     flagStatus,
			ListLogFiles:   []string{"/var/log/apt"},
		}

		wg.Add(1)
		go startHeartbeat(ctx, done, &wg)
		wg.Add(1)
		go startConfigUpdater(ctx, done, &wg)
	}

	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigs
		fmt.Printf("Received signal %s\n", sig)
		close(done)
	}()

	<-done

	fmt.Println("Waiting for sidecars to stop")
	wg.Wait()
	fmt.Println("Done - stopping...")
}
