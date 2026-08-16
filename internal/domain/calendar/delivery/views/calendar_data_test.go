package views

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestApptsAt(t *testing.T) {
	day := WeekDay{
		Appointments: []Appointment{
			{"Dr. Rafael Almeida", "09:00", "09:30", "Ana Souza", StatusConfirmed},
			{"Dr. Rafael Almeida", "09:30", "10:00", "Bruno Lima", StatusPending},
			{"Dr. Rafael Almeida", "10:00", "10:30", "Carla Mendes", StatusCancelled},
			{"Dr. Rafael Almeida", "16:30", "17:00", "Diego Rocha", StatusConfirmed},
		},
	}

	cases := []struct {
		hour int
		half int
		want int
	}{
		{9, 0, 1},
		{9, 1, 1},
		{10, 0, 1},
		{10, 1, 0},
		{16, 1, 1},
		{8, 0, 0},
	}
	for _, tc := range cases {
		if got := len(apptsAt(day, tc.hour, tc.half)); got != tc.want {
			t.Fatalf("apptsAt(%d:%02d) = %d, want %d", tc.hour, tc.half*30, got, tc.want)
		}
	}
}

func TestBuildMonthGrid(t *testing.T) {
	// August 2026: the 1st is a Saturday, 31 days, 42 cells total.
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	grid := BuildMonthGrid(now, map[int][]Appointment{
		15: {{"Dr. Rafael Almeida", "09:00", "09:00", "Ana Souza", StatusConfirmed}},
	})

	if grid.Title != "August 2026" {
		t.Fatalf("title = %q, want %q", grid.Title, "August 2026")
	}
	if len(grid.Days) != 42 {
		t.Fatalf("cells = %d, want 42", len(grid.Days))
	}
	if len(grid.Weekdays) != 7 {
		t.Fatalf("weekdays = %d, want 7", len(grid.Weekdays))
	}

	// Saturday first: six cells with the previous month's last days
	// (July 2026 has 31 days -> 26..31), greyed out, then day 1.
	for i, d := range []int{26, 27, 28, 29, 30, 31, 1, 2} {
		if got := grid.Days[i].Day; got != d {
			t.Fatalf("cell %d = %d, want %d", i, got, d)
		}
		if grid.Days[i].OutOfMonth != (i < 6) {
			t.Fatalf("cell %d OutOfMonth = %v", i, grid.Days[i].OutOfMonth)
		}
	}
	// Day 31 sits at index 36 (6 + 30); the trailing five cells carry
	// the next month's days 1..5, greyed out.
	if got := grid.Days[36].Day; got != 31 || grid.Days[36].OutOfMonth {
		t.Fatalf("cell 36 = %d (out=%v), want 31 in-month", got, grid.Days[36].OutOfMonth)
	}
	for i := 37; i <= 41; i++ {
		if !grid.Days[i].OutOfMonth {
			t.Fatalf("cell %d is not marked out-of-month", i)
		}
	}
	if got := grid.Days[41].Day; got != 5 {
		t.Fatalf("cell 41 = %d, want 5 (next month)", got)
	}

	if !grid.Days[20].IsToday { // day 15
		t.Fatalf("day 15 is not marked as today")
	}
	if got := len(grid.Days[20].Appointments); got != 1 {
		t.Fatalf("day 15 appointments = %d, want 1", got)
	}
}

func TestBuildWeekGrid(t *testing.T) {
	// Aug 15, 2026 is a Saturday: the week runs Aug 9 (Sun) to Aug 15.
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	grid := BuildWeekGrid(now, map[int][]Appointment{
		9:  {{"Dr. Rafael Almeida", "09:00", "09:00", "Ana Souza", StatusConfirmed}},
		15: {{"Dr. Rafael Almeida", "08:00", "08:00", "Bruno Lima", StatusConfirmed}},
	})

	if grid.Title != "Aug 9 — Aug 15, 2026" {
		t.Fatalf("title = %q", grid.Title)
	}
	if len(grid.Days) != 7 {
		t.Fatalf("days = %d, want 7", len(grid.Days))
	}
	if grid.Days[0].Name != "Sun" || grid.Days[6].Name != "Sat" {
		t.Fatalf("week must start on Sunday: %s..%s", grid.Days[0].Name, grid.Days[6].Name)
	}
	if grid.Days[0].Day != 9 || grid.Days[6].Day != 15 {
		t.Fatalf("week days = %d..%d, want 9..15", grid.Days[0].Day, grid.Days[6].Day)
	}
	if !grid.Days[6].IsToday {
		t.Fatalf("Saturday (day 15) is not marked as today")
	}
	if got := len(grid.Days[0].Appointments); got != 1 {
		t.Fatalf("Sunday appointments = %d, want 1", got)
	}
	if grid.StartHour != 8 || grid.EndHour != 18 {
		t.Fatalf("grid range = %d..%d, want 8..18 (clinic hours default)", grid.StartHour, grid.EndHour)
	}
}

func TestGridRangeFollowsAppointments(t *testing.T) {
	// An after-hours slot (telemedicine) extends the visible grid; the
	// end time counts too (20:30-21:30 reaches hour 21).
	grid := BuildWeekGrid(time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC), map[int][]Appointment{
		15: {{"Dr. Rafael Almeida", "09:00", "09:30", "Ana Souza", StatusConfirmed}, {"Dr. Rafael Almeida", "20:30", "21:30", "Sofia Rezende", StatusPending}},
	})
	if grid.StartHour != 8 || grid.EndHour != 21 {
		t.Fatalf("grid range = %d..%d, want 8..21 (clinic + after-hours slot incl. end)", grid.StartHour, grid.EndHour)
	}
}

func TestBuildMonthGridAddsTodayFixture(t *testing.T) {
	// A day without fixtures gets a placeholder so the mock always has
	// content on the highlighted day.
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	grid := BuildMonthGrid(now, map[int][]Appointment{})

	for _, day := range grid.Days {
		if day.IsToday {
			if len(day.Appointments) != 1 {
				t.Fatalf("today has %d appointments, want the placeholder", len(day.Appointments))
			}
			return
		}
	}
	t.Fatalf("no cell marked as today")
}

func TestNowLineOffset(t *testing.T) {
	cases := []struct {
		time string
		want int
	}{
		{"08:00", 40},   // grid top, below the day headers
		{"08:30", 72},   // first half-hour row
		{"10:00", 168},  // two hours in: 40 + 4*32
		{"10:15", 184},  // proportional inside the half hour
		{"07:00", 40},   // clamped to the grid top
		{"23:00", 808},  // clamped: 40 + 24*32 (range 8..20)
	}
	for _, tc := range cases {
		parts := strings.Split(tc.time, ":")
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		now := time.Date(2026, 8, 15, h, m, 0, 0, time.UTC)
		if got := nowLineOffset(now, 8, 20); got != tc.want {
			t.Fatalf("nowLineOffset(%s) = %d, want %d", tc.time, got, tc.want)
		}
	}
}

func TestApptGeometry(t *testing.T) {
	cases := []struct {
		appt   Appointment
		startH int
		top    int
		height int
	}{
		{Appointment{"Dr. Rafael Almeida", "09:00", "09:45", "Ana Souza", StatusConfirmed}, 8, 64, 48},
		{Appointment{"Dr. Rafael Almeida", "20:00", "20:50", "Sofia Rezende", StatusPending}, 8, 768, 53},
		{Appointment{"Dr. Rafael Almeida", "11:00", "11:15", "Diego Rocha", StatusCancelled}, 8, 192, 16},
		{Appointment{"Dr. Rafael Almeida", "08:00", "08:15", "Larissa Gomes", StatusConfirmed}, 8, 0, 16},
	}
	for _, tc := range cases {
		if got := apptTop(tc.appt, tc.startH); got != tc.top {
			t.Fatalf("apptTop(%s) = %d, want %d", tc.appt.Start, got, tc.top)
		}
		if got := apptHeight(tc.appt); got != tc.height {
			t.Fatalf("apptHeight(%s-%s) = %d, want %d", tc.appt.Start, tc.appt.End, got, tc.height)
		}
	}
}
