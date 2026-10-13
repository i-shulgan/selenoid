package service

import (
	"sync"
	"testing"

	"github.com/aerokube/selenoid/session"
	ctr "github.com/moby/moby/api/types/container"
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
