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

package daemon

import (
	"fmt"
	"strings"
	"time"

	"github.com/Graylog2/collector-sidecar/backends"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func CleanOldServices(assignedBackends []*backends.Backend) {
	registeredServices, err := getRegisteredServices()
	if err != nil {
		log.Warn("Failed to get registered services. Skipping cleanup. ", err)
		return
	}
	for _, service := range registeredServices {
		if strings.Contains(service, ServiceNamePrefix()) {
			log.Debugf("Found graylog service %s", service)
			if !serviceIsAssigned(assignedBackends, service) {
				log.Infof("Removing stale graylog service %s", service)
				uninstallService(service)
			}
		}
	}
}

func getRegisteredServices() ([]string, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, err
	}
	defer m.Disconnect()
	registeredServices, err := m.ListServices()
	if err != nil {
		return nil, err
	}
	return registeredServices, nil
}

func serviceIsAssigned(assignedBackends []*backends.Backend, serviceName string) bool {
	for _, backend := range assignedBackends {
		if backend.ServiceType == "svc" && ServiceNamePrefix()+backend.Name == serviceName {
			return true
		}
	}
	return false
}

func uninstallService(name string) {
	log.Infof("Uninstalling service %s", name)
	stopService(name)

	m, err := mgr.Connect()
	if err != nil {
		log.Errorf("Failed to connect to service manager: %v", err)
		return
	}
	defer m.Disconnect()

	s, err := m.OpenService(name)
	// service exist so we try to uninstall it
	if err != nil {
		log.Debugf("Service %s doesn't exist, no uninstall action needed", name)
		return
	}

	defer s.Close()
	err = s.Delete()
	if err != nil {
		log.Errorf("Can't delete service %s: %v", name, err)
	}

	err = eventlog.Remove(s.Name)
	if err != nil {
		log.Errorf("RemoveEventLogSource() failed: %s", err)
	}
	return
}

func stopService(serviceName string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("Failed to connect to service manager: %v", err)
	}
	defer m.Disconnect()

	ws, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("Could not access service %s: %v", serviceName, err)
	}
	defer ws.Close()

	status, err := ws.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("Could not send stop control: %v", err)
	}

	timeout := time.Now().Add(10 * time.Second)
	for status.State != svc.Stopped {
		if timeout.Before(time.Now()) {
			return fmt.Errorf("Timeout waiting for service to go to stopped state: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
		status, err = ws.Query()
		if err != nil {
			return fmt.Errorf("Could not retrieve service status: %v", err)
		}
	}
	return nil
}
