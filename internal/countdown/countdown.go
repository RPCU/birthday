// Package countdown works out when the next birthday starts and ends.
package countdown

import (
	"fmt"
	"strings"
	"time"
)

// Birthday is a date that recurs every year. Year is optional (0 when
// unknown) and is only used to tell how old someone is turning.
type Birthday struct {
	Year  int
	Month time.Month
	Day   int
}

// State describes the current or upcoming birthday relative to a moment.
type State struct {
	// Start is the midnight the birthday begins, End the midnight it ends.
	Start time.Time
	End   time.Time
	// Today is true while the moment falls inside [Start, End).
	Today bool
	// Turning is the age reached on that birthday, 0 when the year is unknown.
	Turning int
}

// Parse reads a birthday written as MM-DD or YYYY-MM-DD.
func Parse(s string) (Birthday, error) {
	s = strings.TrimSpace(s)
	switch len(s) {
	case len("01-02"):
		// Parse against a leap year so 02-29 is accepted.
		t, err := time.Parse(time.DateOnly, "2000-"+s)
		if err != nil {
			return Birthday{}, fmt.Errorf("birthday %q: want MM-DD or YYYY-MM-DD", s)
		}
		return Birthday{Month: t.Month(), Day: t.Day()}, nil
	case len(time.DateOnly):
		t, err := time.Parse(time.DateOnly, s)
		if err != nil {
			return Birthday{}, fmt.Errorf("birthday %q: want MM-DD or YYYY-MM-DD", s)
		}
		return Birthday{Year: t.Year(), Month: t.Month(), Day: t.Day()}, nil
	default:
		return Birthday{}, fmt.Errorf("birthday %q: want MM-DD or YYYY-MM-DD", s)
	}
}

// Occurrence returns the midnight, in loc, that starts the birthday in year.
// A 29 February birthday is celebrated on 28 February in non-leap years.
func (b Birthday) Occurrence(year int, loc *time.Location) time.Time {
	day := b.Day
	if b.Month == time.February && day == 29 && !isLeap(year) {
		day = 28
	}
	return time.Date(year, b.Month, day, 0, 0, 0, 0, loc)
}

// At returns the birthday that is in progress at now, or the next one.
// Days are computed in now's location.
func (b Birthday) At(now time.Time) State {
	loc := now.Location()
	year := now.Year()
	for {
		start := b.Occurrence(year, loc)
		// Built from the calendar date rather than Add(24h) so DST days stay
		// a full calendar day.
		end := time.Date(start.Year(), start.Month(), start.Day()+1, 0, 0, 0, 0, loc)
		if now.Before(end) {
			s := State{Start: start, End: end, Today: !now.Before(start)}
			if b.Year > 0 && year > b.Year {
				s.Turning = year - b.Year
			}
			return s
		}
		year++
	}
}

// Reveal reports whether a surprise tied to the birthday is visible at now:
// from the start of the birthday until days full days after it ends. until is
// the midnight it hides again.
func (b Birthday) Reveal(now time.Time, days int) (open bool, until time.Time) {
	loc := now.Location()
	// The most recent birthday may fall in the previous year (e.g. 24 December
	// seen on 3 January).
	for _, year := range []int{now.Year(), now.Year() - 1} {
		start := b.Occurrence(year, loc)
		until := time.Date(start.Year(), start.Month(), start.Day()+1+days, 0, 0, 0, 0, loc)
		if !now.Before(start) && now.Before(until) {
			return true, until
		}
	}
	return false, time.Time{}
}

// DaysUntil counts the calendar days from now's date to the birthday's date:
// 0 on the day itself, 1 the day before.
func (s State) DaysUntil(now time.Time) int {
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	to := time.Date(s.Start.Year(), s.Start.Month(), s.Start.Day(), 0, 0, 0, 0, time.UTC)
	days := int(to.Sub(from).Hours() / 24)
	return max(days, 0)
}

func isLeap(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}
