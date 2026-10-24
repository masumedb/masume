package cfg

import (
	"testing"
	"time"
)

func TestPasswordCommandTimeoutStopsTheChildrenOfTheCommand(t *testing.T) {
	saved := passwordCommandTimeout
	passwordCommandTimeout = time.Second
	t.Cleanup(func() { passwordCommandTimeout = saved })

	started := time.Now()
	_, err := runPasswordCommand("password command", "shop", "sleep 6 & wait; echo secret")
	elapsed := time.Since(started)

	if err == nil {
		t.Error("a command past its time limit answered no error")
	}
	if elapsed > 4*time.Second {
		t.Errorf("the command returned after %s, wanted the 1s time limit", elapsed)
	}
}
