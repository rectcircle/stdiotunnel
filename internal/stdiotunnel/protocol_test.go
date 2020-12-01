package stdiotunnel

import (
	"math/rand"
	"sort"
	"testing"
	"time"
)

func randomBytes(maxLen int) []byte {
	l := rand.Intn(maxLen)
	result := make([]byte, l)
	for i := 0; i < l; i++ {
		result[i] = byte(rand.Intn(256))
	}
	return result
}

func Test_handleBytes(t *testing.T) {
	t.Run("handleBytes Smoke test", func(t *testing.T) {
		want := NewRequestSegment()
		got := handleBytes(&Segment{}, &segmentState{}, want.Serialize())
		if len(got) != 1 {
			t.Errorf("len(handleBytes()) != 1")
		}
		if !want.Equal(&got[0]) {
			t.Errorf("handleBytes() = %v, want %v", got, want)
		}
	})

	randomTestOne := func(t *testing.T) {
		wants := []Segment{}
		got := []Segment{}
		l := 5 + rand.Intn(10)
		seed := time.Now().UnixNano()
		rand.Seed(seed)
		for i := 0; i < l; i++ {
			t := rand.Intn(5)
			switch t {
			case 0:
				wants = append(wants, NewRequestSegment())
			case 1:
				wants = append(wants, NewAckSegment(uint16(rand.Intn(10000))))
			case 2:
				wants = append(wants, NewSendDataSegment(uint16(rand.Intn(10000)), randomBytes(100)))
			case 3:
				wants = append(wants, NewCloseSegment(uint16(rand.Intn(10000))))
			case 4:
				wants = append(wants, NewHeartbeatSegment())
			}
		}
		buffers := []byte{}
		for _, s := range wants {
			buffers = append(buffers, s.Serialize()...)
		}
		splitIndexes := []int{}
		splitIndexNum := 3 + rand.Intn(8)
		for i := 0; i < splitIndexNum; i++ {
			splitIndexes = append(splitIndexes, rand.Intn(len(buffers)))
		}
		sort.Ints(splitIndexes)
		splitIndexes = append([]int{0}, splitIndexes...)
		splitIndexes = append(splitIndexes, len(buffers))
		cache := Segment{}
		state := segmentState{}
		for i := 1; i < len(splitIndexes); i++ {
			got = append(got, handleBytes(&cache, &state, buffers[splitIndexes[i-1]:splitIndexes[i]])...)
		}
		if len(wants) != len(got) {
			t.Errorf("len(wants) != len(got): %d != %d (seed = %d)", len(wants), len(got), seed)
		}
		for i := 0; i < len(wants); i++ {
			if !wants[i].Equal(&got[i]) {
				t.Errorf("wants[i] != got[i]: %v != %v : (i = %d, seed = %d)", wants[i], got[i], i, seed)
			}
		}
	}

	t.Run("handleBytes Random Test", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			randomTestOne(t)
		}
	})
}
