package discovery

import (
	"log"
	"strconv"

	"github.com/grandcat/zeroconf"

	"nudge/internal/api"
)

// ServiceType is the DNS-SD service type clients browse for.
const ServiceType = "_nudge._tcp"

// register publishes one service type with its TXT records; tests replace it.
var register = func(name, service string, port int, txt []string) (func(), error) {
	server, err := zeroconf.Register(name, service, "local.", port, txt, nil)
	if err != nil {
		return nil, err
	}
	return server.Shutdown, nil
}

// Announce publishes the mDNS service on the LAN under ServiceType with the
// TXT record protocol=<api.ProtocolVersion>. The returned function shuts the
// announcement down; callers must invoke it on exit so the records are
// withdrawn and the registration goroutine stops.
func Announce(name, port string) (shutdown func(), err error) {
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 || p > 65535 {
		return nil, &strconv.NumError{Func: "Atoi", Num: port, Err: strconv.ErrRange}
	}
	stop, err := register(name, ServiceType, p, []string{"protocol=" + strconv.Itoa(api.ProtocolVersion)})
	if err != nil {
		return nil, err
	}
	log.Printf("mDNS: announced %s as %s on port %s", name, ServiceType, port)
	return stop, nil
}
