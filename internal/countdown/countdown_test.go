package countdown

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in      string
		want    Birthday
		wantErr bool
	}{
		{in: "07-20", want: Birthday{Month: time.July, Day: 20}},
		{in: " 02-29 ", want: Birthday{Month: time.February, Day: 29}},
		{in: "1996-07-20", want: Birthday{Year: 1996, Month: time.July, Day: 20}},
		{in: "13-01", wantErr: true},
		{in: "02-30", wantErr: true},
		{in: "1997-02-29", wantErr: true},
		{in: "20/07", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("Parse(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestAt(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	b := Birthday{Year: 1996, Month: time.July, Day: 20}

	tests := []struct {
		name        string
		now         time.Time
		wantStart   time.Time
		wantToday   bool
		wantTurning int
		wantDays    int
	}{
		{
			name:        "earlier in the year",
			now:         time.Date(2026, time.March, 1, 12, 0, 0, 0, paris),
			wantStart:   time.Date(2026, time.July, 20, 0, 0, 0, 0, paris),
			wantTurning: 30,
			wantDays:    141,
		},
		{
			name:        "the day before, late evening",
			now:         time.Date(2026, time.July, 19, 23, 59, 0, 0, paris),
			wantStart:   time.Date(2026, time.July, 20, 0, 0, 0, 0, paris),
			wantTurning: 30,
			wantDays:    1,
		},
		{
			name:        "exactly midnight",
			now:         time.Date(2026, time.July, 20, 0, 0, 0, 0, paris),
			wantStart:   time.Date(2026, time.July, 20, 0, 0, 0, 0, paris),
			wantToday:   true,
			wantTurning: 30,
		},
		{
			name:        "last second of the day",
			now:         time.Date(2026, time.July, 20, 23, 59, 59, 0, paris),
			wantStart:   time.Date(2026, time.July, 20, 0, 0, 0, 0, paris),
			wantToday:   true,
			wantTurning: 30,
		},
		{
			name:        "after the birthday rolls to next year",
			now:         time.Date(2026, time.October, 21, 9, 0, 0, 0, paris),
			wantStart:   time.Date(2027, time.July, 20, 0, 0, 0, 0, paris),
			wantTurning: 31,
			wantDays:    272,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.At(tt.now)
			if !got.Start.Equal(tt.wantStart) {
				t.Errorf("Start = %v, want %v", got.Start, tt.wantStart)
			}
			if !got.End.Equal(tt.wantStart.AddDate(0, 0, 1)) {
				t.Errorf("End = %v, want a day after Start", got.End)
			}
			if got.Today != tt.wantToday {
				t.Errorf("Today = %v, want %v", got.Today, tt.wantToday)
			}
			if got.Turning != tt.wantTurning {
				t.Errorf("Turning = %d, want %d", got.Turning, tt.wantTurning)
			}
			if days := got.DaysUntil(tt.now); days != tt.wantDays {
				t.Errorf("DaysUntil = %d, want %d", days, tt.wantDays)
			}
		})
	}
}

func TestAtLeapDay(t *testing.T) {
	b := Birthday{Month: time.February, Day: 29}

	got := b.At(time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC))
	if want := time.Date(2027, time.February, 28, 0, 0, 0, 0, time.UTC); !got.Start.Equal(want) {
		t.Errorf("non-leap year Start = %v, want %v", got.Start, want)
	}

	got = b.At(time.Date(2028, time.January, 1, 0, 0, 0, 0, time.UTC))
	if want := time.Date(2028, time.February, 29, 0, 0, 0, 0, time.UTC); !got.Start.Equal(want) {
		t.Errorf("leap year Start = %v, want %v", got.Start, want)
	}
}

func TestAtUnknownYear(t *testing.T) {
	b := Birthday{Month: time.July, Day: 20}
	if got := b.At(time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)); got.Turning != 0 {
		t.Errorf("Turning = %d, want 0 when the year is unknown", got.Turning)
	}
}
