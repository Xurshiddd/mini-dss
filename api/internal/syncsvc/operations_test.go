package syncsvc

import (
	"errors"
	"testing"
)

func TestDeviceOperationIsExclusivePerDevice(t *testing.T) {
	s := New(nil, "", nil, nil, nil)
	release, err := s.BeginDeviceOperation(7, "sync")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginDeviceOperation(7, "probe"); !errors.Is(err, ErrDeviceBusy) {
		t.Fatalf("bir qurilmada ikkinchi amal ErrDeviceBusy qaytarishi kerak: %v", err)
	}
	otherRelease, err := s.BeginDeviceOperation(8, "probe")
	if err != nil {
		t.Fatalf("boshqa qurilma bloklanmasligi kerak: %v", err)
	}
	otherRelease()
	release()
	if releaseAgain, err := s.BeginDeviceOperation(7, "probe"); err != nil {
		t.Fatalf("release'dan keyin qurilma bo'sh bo'lishi kerak: %v", err)
	} else {
		releaseAgain()
	}
}
