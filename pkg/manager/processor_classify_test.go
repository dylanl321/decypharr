package manager

import (
	"errors"
	"testing"
)

func TestClassifySubmitError_UnknownIsPermanent(t *testing.T) {
	m := &Manager{}
	reason, shouldPend := m.classifySubmitError(errors.New("invalid api key"), &ImportRequest{})
	if shouldPend {
		t.Fatalf("expected unknown error to be permanent, got pending reason %q", reason)
	}
}
