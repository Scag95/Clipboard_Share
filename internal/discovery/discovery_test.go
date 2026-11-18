package discovery

import "testing"

func TestServiceTypeIsNudge(t *testing.T) {
	if ServiceType != "_nudge._tcp" {
		t.Fatalf("ServiceType = %q, want _nudge._tcp", ServiceType)
	}
}
