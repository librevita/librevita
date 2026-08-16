package views

import (
	"strconv"
	"strings"
	"time"
)

// Clinic hours are the default visible grid, both inclusive. The range
// extends beyond them whenever appointments fall before or after the
// official schedule (e.g. telemedicine after closing): the calendar
// always covers [clinic hours] ∪ [appointment hours].
const (
	ClinicStartHour = 8
	ClinicEndHour   = 18
)

// appointmentSlot returns the half-hour slot index of a time string
// ("09:30" -> hour 9, half 1); ok is false for malformed times.
func appointmentSlot(t string) (hour int, half int, ok bool) {
	parts := strings.Split(t, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m / 30, true
}

// apptsAt filters the day's appointments to one half-hour row.
func apptsAt(day WeekDay, hour int, half int) []Appointment {
	var out []Appointment
	for _, a := range day.Appointments {
		h, hf, ok := appointmentSlot(a.Start)
		if ok && h == hour && hf == half {
			out = append(out, a)
		}
	}
	return out
}

// apptTop returns the pixel offset of the appointment start from the
// top of the day's time area (below the day header), at 32px per
// half-hour.
func apptTop(a Appointment, gridStartHour int) int {
	m, ok := parseMinutes(a.Start)
	if !ok {
		return 0
	}
	return (m - gridStartHour*60) * 32 / 30
}

// apptHeight returns the pixel height of the appointment, proportional
// to its exact duration (32px per half-hour). Malformed times fall back
// to one half-hour.
func apptHeight(a Appointment) int {
	s, ok1 := parseMinutes(a.Start)
	e, ok2 := parseMinutes(a.End)
	if !ok1 || !ok2 {
		return 32
	}
	return (e - s) * 32 / 30
}

// parseMinutes returns the total minutes since midnight; ok is false
// for malformed times. Unlike appointmentSlot it keeps the exact
// minutes, which the appointment geometry needs.
func parseMinutes(t string) (int, bool) {
	parts := strings.Split(t, ":")
	if len(parts) != 2 {
		return 0, false
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// AppointmentStatus is the mock status of a schedule entry.
type AppointmentStatus string

const (
	StatusConfirmed AppointmentStatus = "confirmed"
	StatusPending   AppointmentStatus = "pending"
	StatusCancelled AppointmentStatus = "cancelled"
)

// Appointment is a static fixture: physician, start/end times and
// patient label.
type Appointment struct {
	Physician string // "Dr. Rafael Almeida"
	Start     string // "09:15"
	End       string // "09:45"
	Patient   string
	Status    AppointmentStatus
}

// Physicians is the mock medical staff, rendered in the filter select.
var Physicians = []string{
	"Dr. Rafael Almeida",
	"Dra. Beatriz Sousa",
	"Dr. Carlos Eduardo Lima",
	"Dr. Marina Costa",
}

// DayCell is one calendar slot. OutOfMonth marks the leading/trailing
// cells that show the adjacent month's days, rendered greyed out.
type DayCell struct {
	Day          int
	IsToday      bool
	OutOfMonth   bool
	Appointments []Appointment
}

// WeekDay is one column of the weekly view.
type WeekDay struct {
	Name         string // "Sun"
	Day          int
	IsToday      bool
	Appointments []Appointment
}

// WeekGrid is the weekly calendar data the template renders. StartHour
// and EndHour delimit the visible grid, derived from the week's
// appointments: the schedule follows the providers' actual hours
// (official shifts plus telemedicine after closing), not a fixed
// office-hours block. NowOffset is the pixel offset of the current time
// from the grid top (day header included), clamped to that range for
// the mock. The layout constants mirror the template: h-10 header
// (40px) and h-8 half-hour rows (32px).
type WeekGrid struct {
	Title     string
	Days      []WeekDay
	StartHour int
	EndHour   int
	NowOffset int
}

// gridRange returns the inclusive hour range covering the clinic hours
// plus every appointment of the week (start and end), so after-hours
// slots extend the grid beyond the official schedule.
func gridRange(days []WeekDay) (start int, end int) {
	start, end = ClinicStartHour, ClinicEndHour
	for _, day := range days {
		for _, a := range day.Appointments {
			for _, t := range []string{a.Start, a.End} {
				h, _, ok := appointmentSlot(t)
				if !ok {
					continue
				}
				if h < start {
					start = h
				}
				if h > end {
					end = h
				}
			}
		}
	}
	return start, end
}

// nowLineOffset computes the now-line position in pixels. The header is
// 40px tall and each half-hour row 32px; the offset is proportional
// inside the current half hour. Outside the visible range the line is
// clamped to the grid edges (mock behavior: the demo must always show
// the line; the real calendar would hide it).
func nowLineOffset(now time.Time, start int, end int) int {
	const headerPx = 40
	const halfHourPx = 32
	minutes := now.Hour()*60 + now.Minute()
	from := start * 60
	to := end * 60
	if minutes < from {
		minutes = from
	}
	if minutes > to {
		minutes = to
	}
	return headerPx + (minutes-from)*halfHourPx/30
}

// MonthGrid is the monthly calendar data the template renders.
type MonthGrid struct {
	Title    string
	Weekdays []string
	Days     []DayCell
}

// Fixtures is the mock schedule, keyed by day of month. The patient
// names are placeholders. The 20:00 slot models telemedicine after the
// official closing time: providers (e.g. psychologists) keep seeing
// patients when the reception desk is already closed.
var Fixtures = map[int][]Appointment{
	2:  {{"Dr. Rafael Almeida", "09:15", "09:45", "Ana Souza", StatusConfirmed}},
	5:  {{"Dr. Rafael Almeida", "10:30", "11:00", "Bruno Lima", StatusConfirmed}, {"Dra. Beatriz Sousa", "14:45", "15:25", "Carla Mendes", StatusPending}},
	8:  {{"Dra. Beatriz Sousa", "11:00", "11:15", "Diego Rocha", StatusCancelled}, {"Dr. Carlos Eduardo Lima", "16:15", "17:05", "Eduardo Pires", StatusConfirmed}},
	12: {{"Dr. Rafael Almeida", "08:30", "09:00", "Elisa Faria", StatusConfirmed}, {"Dr. Carlos Eduardo Lima", "16:30", "17:00", "Felipe Nunes", StatusPending}},
	15: {{"Dra. Beatriz Sousa", "09:00", "09:45", "Gabriela Castro", StatusConfirmed}, {"Dr. Rafael Almeida", "13:30", "13:50", "Henrique Dias", StatusConfirmed}, {"Dr. Marina Costa", "20:00", "20:50", "Sofia Rezende", StatusPending}},
	19: {{"Dra. Beatriz Sousa", "10:00", "10:30", "Iara Pinto", StatusPending}, {"Dr. Marina Costa", "11:45", "12:25", "Júlia Ramos", StatusConfirmed}},
	22: {{"Dr. Carlos Eduardo Lima", "15:00", "15:45", "João Teixeira", StatusCancelled}, {"Dr. Marina Costa", "17:15", "17:45", "Karina Lopes", StatusPending}},
	26: {{"Dr. Rafael Almeida", "08:00", "08:15", "Larissa Gomes", StatusConfirmed}, {"Dr. Carlos Eduardo Lima", "12:00", "12:45", "Marcos Vieira", StatusConfirmed}, {"Dr. Marina Costa", "17:00", "18:00", "Nina Duarte", StatusPending}},
	29: {{"Dr. Rafael Almeida", "09:30", "10:00", "Otávio Barros", StatusConfirmed}, {"Dra. Beatriz Sousa", "15:45", "16:35", "Paula Freitas", StatusConfirmed}},
}

// BuildWeekGrid computes the week around now, Sunday to Saturday. The
// appointment fixtures are reused from the month view.
func BuildWeekGrid(now time.Time, fixtures map[int][]Appointment) WeekGrid {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -int(now.Weekday()))

	var days []WeekDay
	for i := 0; i < 7; i++ {
		d := start.AddDate(0, 0, i)
		days = append(days, WeekDay{
			Name:         d.Format("Mon"),
			Day:          d.Day(),
			IsToday:      d.Day() == now.Day() && d.Month() == now.Month(),
			Appointments: withTodayFixture(now, d.Day(), fixtures[d.Day()]),
		})
	}
	end := start.AddDate(0, 0, 6)
	gridStart, gridEnd := gridRange(days)
	return WeekGrid{
		Title:     start.Format("Jan 2") + " — " + end.Format("Jan 2, 2006"),
		Days:      days,
		StartHour: gridStart,
		EndHour:   gridEnd,
		NowOffset: nowLineOffset(now, gridStart, gridEnd),
	}
}

// withTodayFixture appends the placeholder appointment when a day has
// none, so the mock always shows content on the highlighted day.
func withTodayFixture(now time.Time, day int, row []Appointment) []Appointment {
	if day == now.Day() && len(row) == 0 {
		return append(row, Appointment{"Dr. Rafael Almeida", "10:00", "10:30", "Novo paciente", StatusPending})
	}
	return row
}

// BuildMonthGrid computes the month grid around now. The first day of
// the week is Sunday, matching the datepicker; the leading and trailing
// cells carry the adjacent month's days.
func BuildMonthGrid(now time.Time, fixtures map[int][]Appointment) MonthGrid {
	year, month := now.Year(), now.Month()
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	prevDays := time.Date(year, month, 0, 0, 0, 0, 0, time.UTC).Day()
	leading := int(first.Weekday())

	var days []DayCell
	for i := 0; i < leading; i++ {
		days = append(days, DayCell{Day: prevDays - leading + 1 + i, OutOfMonth: true})
	}
	for d := 1; d <= daysInMonth; d++ {
		days = append(days, DayCell{Day: d, IsToday: d == now.Day(),
			Appointments: withTodayFixture(now, d, fixtures[d])})
	}
	next := 1
	for len(days)%7 != 0 {
		days = append(days, DayCell{Day: next, OutOfMonth: true})
		next++
	}

	return MonthGrid{
		Title:    first.Format("January 2006"),
		Weekdays: []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"},
		Days:     days,
	}
}

// chipShow renders the physician-filter x-show expression for one
// appointment as a pure literal comparison: the Alpine CSP build's
// evaluator is only relied on for literals and comparisons (the
// datepicker profile), never for $el/dataset magic.
func chipShow(physician string) string {
	return "physician === '' || physician === '" + physician + "'"
}

// apptStyle renders the appointment block style binding as a full
// literal, with the geometry computed by the server.
func apptStyle(a Appointment, gridStartHour int) string {
	return "'top:" + strconv.Itoa(apptTop(a, gridStartHour)) + "px;height:" + strconv.Itoa(apptHeight(a)) + "px'"
}

// nowLineStyle renders the now-line style binding as a full literal.
func nowLineStyle(offset int) string {
	return "'top:" + strconv.Itoa(offset) + "px'"
}

// patientLineClass hides the patient line on blocks shorter than half
// an hour, computed by the server (no client-side comparison).
func patientLineClass(a Appointment) string {
	base := "truncate text-[10px] leading-3"
	if apptHeight(a) < 32 {
		return base + " hidden"
	}
	return base
}

// chipShowCall renders the physician-filter x-show expression as a
// method call with a literal argument — the datepicker profile — never
// as an inline comparison expression.
func chipShowCall(physician string) string {
	return "showChip('" + physician + "')"
}
