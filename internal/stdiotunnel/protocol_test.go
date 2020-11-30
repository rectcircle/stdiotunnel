package stdiotunnel

import (
	"testing"
)

func Test_handleBytes(t *testing.T) {
	t.Run("Random Test handleBytes", func(t *testing.T) {
		want := NewRequestSegment()
		got := handleBytes(&Segment{}, &segmentState{}, want.Serialize())
		if len(got) != 1 {
			t.Errorf("len(handleBytes()) != 1")
		}
		if !want.Equal(&got[0]) {
			t.Errorf("handleBytes() = %v, want %v", got, want)
		}
	})
}
