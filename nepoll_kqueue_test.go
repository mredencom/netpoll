package netpoll

import (
	"errors"
	"testing"
	"time"
)

func TestKqueueCreate(t *testing.T) {
	k, err := KqueueCreate()
	if err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 1)
	go k.Wait(func(fd int, event Event) {
		if fd != -1 {
			errs <- errors.New("fd should be -1")
		}
	})

	time.Sleep(time.Millisecond * 500)

	if err = k.Close(); err != nil {
		t.Fatal(err)
	}
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
