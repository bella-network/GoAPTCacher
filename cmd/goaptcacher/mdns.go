package main

import (
	"log"

	"github.com/grandcat/zeroconf"
)

func mDNSAnnouncement() {
	// Create a service entry
	_, err := zeroconf.Register("GoAPTCacher", "_apt_proxy._tcp", "local.", config.ListenPort, nil, nil)
	if err != nil {
		log.Println("[ERR:mDNS] Failed to register service: ", err)
		return
	}
}
