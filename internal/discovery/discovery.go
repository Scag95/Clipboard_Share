package discovery

import (
	"log"
	"strconv"

	"github.com/grandcat/zeroconf"
)

// Announce publishes the mDNS service on the LAN so clients can discover it.
// The returned function shuts the announcement down; callers must invoke it on
// exit so the record is withdrawn and the registration goroutine stops.
func Announce(name, port string) (shutdown func(), err error) {
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 || p > 65535 {
		return nil, &strconv.NumError{Func: "Atoi", Num: port, Err: strconv.ErrRange}
	}
	server, err := zeroconf.Register(name, "_clipboard-share._tcp", "local.", p, []string{"version=1"}, nil)
	if err != nil {
		return nil, err
	}
	log.Printf("mDNS: announced %s on port %s", name, port)
	return func() { server.Shutdown() }, nil
}
