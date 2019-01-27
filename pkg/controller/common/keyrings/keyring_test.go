package keyrings

import (
	"testing"
	"time"
)

func TestEncodeKey(t *testing.T) {
	s, err := EncodeKey([]byte{0x22, 0x8f, 0xba, 0x7d, 0xe7, 0x49, 0xed, 0x34, 0xb3, 0x38, 0xaa, 0x00, 0xc3, 0xa2, 0x2f, 0x9a},
		time.Unix(1546553005, 75622))
	if err != nil {
		t.Fatal(err)
	}
	expected := "AQCthi5cZicBABAAIo+6fedJ7TSzOKoAw6Ivmg=="
	if s != expected {
		t.Errorf("Encoded secret got '%s' expected '%s'", s, expected)
	}
}
