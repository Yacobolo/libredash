package instancelock

import "testing"

func TestAcquireRejectsSecondProcessForSameHome(t *testing.T) {
	home := t.TempDir()
	first, err := Acquire(home)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	if _, err := Acquire(home); err == nil {
		t.Fatal("second lock acquisition succeeded")
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := Acquire(home)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()
}
