package log

import (
	"testing"
	"time"
)

func TestEventsBelowActiveLevelAreDropped(t *testing.T) {
	oldLevel := Level()
	defer SetLevel(oldLevel)

	SetLevel(WARNING)
	subscription := Subscribe()
	defer UnSubscribe(subscription)

	Debugln("hidden debug %d", 1)
	Infoln("hidden info %d", 2)

	select {
	case event := <-subscription:
		t.Fatalf("received dropped event: %#v", event)
	case <-time.After(10 * time.Millisecond):
	}

	Warnln("visible warning")
	select {
	case event := <-subscription:
		if event.Payload != "visible warning" {
			t.Fatalf("payload = %q", event.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("warning event was not delivered")
	}
}
