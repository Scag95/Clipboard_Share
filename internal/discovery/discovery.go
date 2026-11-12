package discovery

import (
	"log"

	"github.com/grandcat/zeroconf"
)

// Announce publica el servicio mDNS en la LAN para que los clientes lo descubran.
func Announce(name, port string) error {
	server, err := zeroconf.Register(name, "_clipboard-share._tcp", "local.", mustAtoi(port), []string{"version=1"}, nil)
	if err != nil {
		return err
	}
	log.Printf("mDNS: announced %s on port %s", name, port)
	// Mantener el registro vivo mientras el proceso corra; se cierra al salir.
	go func() {
		<-make(chan struct{})
		server.Shutdown()
	}()
	return nil
}

func mustAtoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			log.Fatalf("invalid port: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		log.Fatalf("invalid port: %s", s)
	}
	return n
}

