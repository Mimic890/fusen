package callback

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
)

func TestEncode(t *testing.T) {
	cases := []struct {
		data Data
		want string
	}{
		{New("h"), "h"},
		{New("p", int64(42)), "p:16"},
		{New("p", int64(1295), "del", "y"), "p:zz:del:y"},
		{New("s", "lang", "ru"), "s:lang:ru"},
		{New("t", 36, "mv", "r"), "t:10:mv:r"},
	}
	for _, c := range cases {
		got, err := c.data.Encode()
		if err != nil {
			t.Errorf("%+v: %v", c.data, err)
			continue
		}
		if got != c.want {
			t.Errorf("%+v: got %q, want %q", c.data, got, c.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	ids := []int64{1, 35, 36, 1 << 20, math.MaxInt64}
	for _, id := range ids {
		d := New("t", id, "mv", "3")
		s, err := d.Encode()
		if err != nil {
			t.Fatalf("encode %d: %v", id, err)
		}
		got, err := Decode(s)
		if err != nil {
			t.Fatalf("decode %q: %v", s, err)
		}
		if got.Kind != d.Kind || !slices.Equal(got.Args, d.Args) {
			t.Fatalf("round trip mismatch: %+v vs %+v", got, d)
		}
		back, err := got.ID(0)
		if err != nil || back != id {
			t.Fatalf("id: got %d, %v; want %d", back, err, id)
		}
	}
}

func TestMaxInt64FitsWithRoom(t *testing.T) {
	// Worst realistic payload: several max IDs plus actions.
	s, err := New("t", int64(math.MaxInt64), "mv", int64(math.MaxInt64), "y").Encode()
	if err != nil {
		t.Fatal(err)
	}
	if len(s) > MaxLen {
		t.Fatalf("len %d", len(s))
	}
}

func TestEncodeErrors(t *testing.T) {
	cases := []struct {
		name string
		data Data
		want error
	}{
		{"empty kind", Data{}, ErrEmpty},
		{"separator in arg", New("p", "a:b"), ErrBadPart},
		{"empty arg", New("p", ""), ErrBadPart},
		{"too long", New("p", strings.Repeat("x", 63)), ErrTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.data.Encode(); !errors.Is(err, c.want) {
				t.Errorf("got %v, want %v", err, c.want)
			}
		})
	}
}

func TestExactlyMaxLen(t *testing.T) {
	s, err := New("p", strings.Repeat("x", MaxLen-2)).Encode()
	if err != nil || len(s) != MaxLen {
		t.Fatalf("len %d, err %v", len(s), err)
	}
}

func TestDecodeErrors(t *testing.T) {
	for _, s := range []string{"", ":", "p:", "p::x", strings.Repeat("a", MaxLen+1)} {
		if _, err := Decode(s); err == nil {
			t.Errorf("Decode(%q): expected error", s)
		}
	}
}

func TestIDErrors(t *testing.T) {
	d, _ := Decode("p:!!:0:-1")
	for i := range 4 {
		if _, err := d.ID(i); err == nil {
			t.Errorf("ID(%d): expected error", i)
		}
	}
}

func TestStringPanicsOnInvalid(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	_ = New("p", "a:b").String()
}
