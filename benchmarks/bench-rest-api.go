// Copyright (C) 2020 Graylog, Inc.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the Server Side Public License, version 1,
// as published by MongoDB, Inc.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// Server Side Public License for more details.
//
// You should have received a copy of the Server Side Public License
// along with this program. If not, see
// <http://www.mongodb.com/licensing/server-side-public-license>.

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
	"log"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

var flagNum int
var flagInterval int
var flagUrl string
var flagApiToken string
var flagStatus bool
var flagIterations int

func init() {
	flag.IntVar(&flagNum, "num", 10, "number of sidecar processes")
	flag.IntVar(&flagInterval, "interval", 1, "update interval")
	flag.StringVar(&flagUrl, "url", "http://127.0.0.1:9000/api/", "graylog server url")
	flag.StringVar(&flagApiToken, "apitoken", "", "graylog server api token")
	flag.BoolVar(&flagStatus, "status", true, "send status")
	flag.IntVar(&flagIterations, "iterations", 10, "number of request iterations")
	flag.Parse()
}

// XXX
// There is a couple of variables we could tune, to get a real-life benchmark:
// DisableKeepAlives, MaxIdleConnections, DefaultMaxIdleConnsPerHost
var httpClient = rest.NewHTTPClient(&tls.Config{})

func startHeartbeat(ctx *context.Ctx, done chan bool, metrics chan time.Duration, wg *sync.WaitGroup) {
	fmt.Printf("[%s] starting heartbeat\n", ctx.UserConfig.NodeId)
	defer wg.Done()
	for i := 1; i <= flagIterations; i++ {
		select {
		case <-done:
			fmt.Printf("[%s] stopping heartbeat\n", ctx.UserConfig.NodeId)
			return
		default:
			time.Sleep(time.Duration(ctx.UserConfig.UpdateInterval) * time.Second)
			statusRequest := api.NewStatusRequest()
			t := time.Now()
			response, err := api.UpdateRegistration(httpClient, "nochecksum", ctx, &statusRequest)
			if err != nil {
				fmt.Printf("[%s] can't register sidecar: %v\n", ctx.UserConfig.NodeId, err)
				return
			}
			metrics <- time.Since(t)

			// fetch assigned configurations
			for _, ass := range response.Assignments {
				t := time.Now()
				_, err := api.RequestConfiguration(httpClient, ass.ConfigurationId, "nochecksum", ctx)
				if err != nil {
					fmt.Printf("[%s] can't fetch config %s from Graylog API: %v\n", ctx.UserConfig.NodeId, ass.ConfigurationId, err)
					return
				}
				metrics <- time.Since(t)
			}
		}
	}
}

func startBackendUpdater(ctx *context.Ctx, done chan bool, metrics chan time.Duration, wg *sync.WaitGroup) {
	fmt.Printf("[%s] starting backend updater\n", ctx.UserConfig.NodeId)
	defer wg.Done()
	for i := 1; i <= flagIterations; i++ {
		select {
		case <-done:
			fmt.Printf("[%s] stopping backend updater\n", ctx.UserConfig.NodeId)
			return
		default:
			time.Sleep(time.Duration(ctx.UserConfig.UpdateInterval) * time.Second)
			t := time.Now()
			_, err := api.RequestBackendList(httpClient, "nochecksum", ctx)
			if err != nil {
				fmt.Printf("[%s] can't fetch backend from Graylog API: %v\n", ctx.UserConfig.NodeId, err)
				return
			}
			metrics <- time.Since(t)
		}
	}
}

func metricsReceiver(metrics chan time.Duration, done chan bool) {
	durations := []time.Duration{}

	for {
		select {
		case <-done:
			sort.Slice(durations, func(i, j int) bool { return durations[i] > durations[j] })
			sum := time.Duration(0)
			for _, dur := range durations {
				sum += dur
			}
			avg := sum / time.Duration(len(durations))
			fmt.Printf("%d requests. avg: %v max: %v min: %v\n", len(durations), avg, durations[0], durations[len(durations)-1])
			close(metrics)
			return
		case duration := <-metrics:
			durations = append(durations, duration)

		}
	}
}

func main() {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool)
	metrics := make(chan time.Duration)
	var wg sync.WaitGroup

	if flagApiToken == "" {
		log.Fatal("Need server API token")
	}
	go metricsReceiver(metrics, done)

	common.CollectorVersion = "1.0.0"

	for i := 1; i <= flagNum; i++ {
		pid := i

		ctx := context.NewContext()
		ctx.ServerUrl, _ = url.Parse(flagUrl)
		ctx.NodeId = fmt.Sprintf("sidecar-benchmark-%03d", pid)
		ctx.NodeName = fmt.Sprintf("sidecar-benchmark-%03d", pid)
		ctx.UserConfig = &cfgfile.SidecarConfig{
			NodeId:         fmt.Sprintf("sidecar-benchmark-%03d", pid),
			NodeName:       fmt.Sprintf("sidecar-benchmark-%03d", pid),
			ServerApiToken: flagApiToken,
			UpdateInterval: flagInterval,
			SendStatus:     flagStatus,
			ListLogFiles:   []string{"/var/log/apt"},
		}

		wg.Add(1)
		go startHeartbeat(ctx, done, metrics, &wg)
		wg.Add(1)
		go startBackendUpdater(ctx, done, metrics, &wg)
	}

	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigs
		fmt.Printf("Received signal %s\n", sig)
		close(done)
	}()
	go func() {
		wg.Wait()
		fmt.Printf("Finished\n")
		close(done)
	}()

	<-done

	fmt.Println("Waiting for sidecars to stop")
	wg.Wait()
	fmt.Println("Done - stopping...")
	<-metrics
}
