package service

import (
	"net/netip"
	"sync"
	"testing"

	"github.com/aerokube/selenoid/session"
	ctr "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	assert "github.com/stretchr/testify/require"
)

func TestGetLogConfigDoesNotModifySharedConfig(t *testing.T) {
	shared := ctr.LogConfig{Type: "syslog", Config: map[string]string{"syslog-address": "udp://1.2.3.4:514"}}

	first := getLogConfig(shared, session.Caps{TestName: "first-test", Labels: map[string]string{"a": "b"}})
	second := getLogConfig(shared, session.Caps{TestName: "second-test"})

	assert.Equal(t, "first-test", first.Config["tag"])
	assert.Equal(t, "a=b", first.Config["labels"])
	assert.Equal(t, "second-test", second.Config["tag"], "every container must get its own test name")
	_, hasLabels := second.Config["labels"]
	assert.False(t, hasLabels)
	assert.Equal(t, map[string]string{"syslog-address": "udp://1.2.3.4:514"}, shared.Config, "loaded configuration must stay unchanged")

	configured := ctr.LogConfig{Type: "syslog", Config: map[string]string{"tag": "fixed"}}
	assert.Equal(t, "fixed", getLogConfig(configured, session.Caps{TestName: "x"}).Config["tag"], "an explicit tag wins")
}

func TestGetLogConfigConcurrently(t *testing.T) {
	shared := ctr.LogConfig{Type: "syslog", Config: map[string]string{}}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = getLogConfig(shared, session.Caps{TestName: "test", Labels: map[string]string{"k": "v"}})
		}()
	}
	wg.Wait()
}

func TestHostPortWithoutBindings(t *testing.T) {
	selenium := network.MustParsePort("4444/tcp")
	vnc := network.MustParsePort("5900/tcp")
	stat := ctr.InspectResponse{NetworkSettings: &ctr.NetworkSettings{Ports: network.PortMap{
		selenium: {{HostPort: "32768"}},
		vnc:      nil,
	}}}
	pc := map[string]network.Port{"4444": selenium, ports.VNC: vnc}
	hp := getHostPort(Environment{IP: "10.0.0.1"}, "4444", session.Caps{VNC: true}, stat, pc)
	assert.Equal(t, "10.0.0.1:32768", hp.Selenium)
	assert.Equal(t, "", hp.VNC, "a port without bindings must not panic")
	assert.Equal(t, "", hp.Devtools)
	assert.Equal(t, map[string]string{"4444": "32768"}, getContainerPorts(stat))
	assert.Empty(t, getContainerPorts(ctr.InspectResponse{}))
}

func TestGetContainerIP(t *testing.T) {
	stat := ctr.InspectResponse{NetworkSettings: &ctr.NetworkSettings{Networks: map[string]*network.EndpointSettings{
		"bridge": {IPAddress: netip.MustParseAddr("172.17.0.2")},
		"custom": {IPAddress: netip.MustParseAddr("10.1.0.5")},
	}}}
	assert.Equal(t, "172.17.0.2", getContainerIP(DefaultContainerNetwork, stat))
	assert.Equal(t, "10.1.0.5", getContainerIP("custom", stat))
	assert.Equal(t, "10.1.0.5", getContainerIP("missing", stat), "falls back to a known address")
	assert.Equal(t, "", getContainerIP("custom", ctr.InspectResponse{}))
}

func TestParseDNSServers(t *testing.T) {
	addrs, err := parseDNSServers([]string{"8.8.8.8", "2001:4860:4860::8888"})
	assert.NoError(t, err)
	assert.Len(t, addrs, 2)
	_, err = parseDNSServers([]string{"not-an-ip"})
	assert.Error(t, err)
}
