package stream

import "testing"

func TestBrokerUnsubscribeRemovesSubscriber(t *testing.T) {
	broker := NewBroker()
	_, unsubscribe := broker.Subscribe("client:dashboard:page")

	if got := broker.SubscriberCount("client:dashboard:page"); got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}

	unsubscribe()

	if got := broker.SubscriberCount("client:dashboard:page"); got != 0 {
		t.Fatalf("subscriber count after unsubscribe = %d, want 0", got)
	}
}
