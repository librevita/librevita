package views

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.Len(t, apptsAt(day, tc.hour, tc.half), tc.want)
	}
}

func TestBuildMonthGrid(t *testing.T) {
	// August 2026: the 1st is a Saturday, 31 days, 42 cells total.
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	grid := BuildMonthGrid(now, map[int][]Appointment{
		15: {{"Dr. Rafael Almeida", "09:00", "09:00", "Ana Souza", StatusConfirmed}},
	})

	assert.Equal(t, "August 2026", grid.Title)
	assert.Len(t, grid.Days, 42)
	assert.Len(t, grid.Weekdays, 7)

	// Saturday first: six cells with the previous month's last days
	// (July 2026 has 31 days -> 26..31), greyed out, then day 1.
	for i, d := range []int{26, 27, 28, 29, 30, 31, 1, 2} {
		assert.Equal(t, d, grid.Days[i].Day)
		assert.Equal(t, i < 6, grid.Days[i].OutOfMonth)
	}
	// Day 31 sits at index 36 (6 + 30); the trailing five cells carry
	// the next month's days 1..5, greyed out.
	assert.Equal(t, 31, grid.Days[36].Day)
	assert.False(t, grid.Days[36].OutOfMonth)

	for i := 37; i <= 41; i++ {
		assert.True(t, grid.Days[i].OutOfMonth)
	}
	assert.Equal(t, 5, grid.Days[41].Day)

	assert.True(t, grid.Days[20].IsToday) // day 15
	assert.Len(t, grid.Days[20].Appointments, 1)
}

func TestBuildWeekGrid(t *testing.T) {
	// Aug 15, 2026 is a Saturday: the week runs Aug 9 (Sun) to Aug 15.
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	grid := BuildWeekGrid(now, map[int][]Appointment{
		9:  {{"Dr. Rafael Almeida", "09:00", "09:00", "Ana Souza", StatusConfirmed}},
		15: {{"Dr. Rafael Almeida", "08:00", "08:00", "Bruno Lima", StatusConfirmed}},
	})

	assert.Equal(t, "Aug 9 — Aug 15, 2026", grid.Title)
	assert.Len(t, grid.Days, 7)
	assert.Equal(t, "Sun", grid.Days[0].Name)
	assert.Equal(t, "Sat", grid.Days[6].Name)
	assert.Equal(t, 9, grid.Days[0].Day)
	assert.Equal(t, 15, grid.Days[6].Day)
	assert.True(t, grid.Days[6].IsToday)
	assert.Len(t, grid.Days[0].Appointments, 1)
	assert.Equal(t, 8, grid.StartHour)
	assert.Equal(t, 18, grid.EndHour)
}

func TestGridRangeFollowsAppointments(t *testing.T) {
	// An after-hours slot (telemedicine) extends the visible grid; the
	// end time counts too (20:30-21:30 reaches hour 21).
	grid := BuildWeekGrid(time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC), map[int][]Appointment{
		15: {{"Dr. Rafael Almeida", "09:00", "09:30", "Ana Souza", StatusConfirmed}, {"Dr. Rafael Almeida", "20:30", "21:30", "Sofia Rezende", StatusPending}},
	})
	assert.Equal(t, 8, grid.StartHour)
	assert.Equal(t, 21, grid.EndHour)
}

func TestBuildMonthGridAddsTodayFixture(t *testing.T) {
	// A day without fixtures gets a placeholder so the mock always has
	// content on the highlighted day.
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	grid := BuildMonthGrid(now, map[int][]Appointment{})

	foundToday := false
	for _, day := range grid.Days {
		if day.IsToday {
			foundToday = true
			assert.Len(t, day.Appointments, 1)
			break
		}
	}
	require.True(t, foundToday, "no cell marked as today")
}

func TestNowLineOffset(t *testing.T) {
	cases := []struct {
		time string
		want int
	}{
		{"08:00", 40},  // grid top, below the day headers
		{"08:30", 72},  // first half-hour row
		{"10:00", 168}, // two hours in: 40 + 4*32
		{"10:15", 184}, // proportional inside the half hour
		{"07:00", 40},  // clamped to the grid top
		{"23:00", 808}, // clamped: 40 + 24*32 (range 8..20)
	}
	for _, tc := range cases {
		parts := strings.Split(tc.time, ":")
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		now := time.Date(2026, 8, 15, h, m, 0, 0, time.UTC)
		assert.Equal(t, tc.want, nowLineOffset(now, 8, 20), "time: %s", tc.time)
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
		assert.Equal(t, tc.top, apptTop(tc.appt, tc.startH), "apptTop(%s)", tc.appt.Start)
		assert.Equal(t, tc.height, apptHeight(tc.appt), "apptHeight(%s-%s)", tc.appt.Start, tc.appt.End)
	}
}
