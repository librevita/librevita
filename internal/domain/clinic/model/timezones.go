package model

import "time"

// TimeZoneGroup groups IANA zones by region for the onboarding select.
type TimeZoneGroup struct {
	Label string
	Zones []string
}

// TimeZones is the curated list of selectable timezones. It is not exhaustive;
// submissions are validated against the embedded tzdata regardless.
var TimeZones = []TimeZoneGroup{
	{
		Label: "Africa",
		Zones: []string{
			"Africa/Abidjan", "Africa/Accra", "Africa/Algiers", "Africa/Cairo",
			"Africa/Casablanca", "Africa/Johannesburg", "Africa/Kinshasa",
			"Africa/Lagos", "Africa/Nairobi", "Africa/Tunis",
		},
	},
	{
		Label: "America",
		Zones: []string{
			"America/Argentina/Buenos_Aires", "America/Asuncion", "America/Bahia",
			"America/Belem", "America/Bogota", "America/Caracas", "America/Cayenne",
			"America/Chicago", "America/Cuiaba", "America/Denver", "America/Detroit",
			"America/Fortaleza", "America/Halifax", "America/Lima", "America/Los_Angeles",
			"America/Maceio", "America/Manaus", "America/Mexico_City", "America/Montevideo",
			"America/New_York", "America/Noronha", "America/Panama", "America/Phoenix",
			"America/Porto_Velho", "America/Puerto_Rico", "America/Recife", "America/Rio_Branco",
			"America/Santiago", "America/Sao_Paulo", "America/Santarem", "America/Toronto",
			"America/Vancouver",
		},
	},
	{
		Label: "Asia",
		Zones: []string{
			"Asia/Bangkok", "Asia/Beirut", "Asia/Dubai", "Asia/Hong_Kong",
			"Asia/Jakarta", "Asia/Jerusalem", "Asia/Kolkata", "Asia/Kuala_Lumpur",
			"Asia/Manila", "Asia/Riyadh", "Asia/Seoul", "Asia/Shanghai",
			"Asia/Singapore", "Asia/Taipei", "Asia/Tokyo",
		},
	},
	{
		Label: "Atlantic",
		Zones: []string{
			"Atlantic/Azores", "Atlantic/Canary", "Atlantic/Cape_Verde",
			"Atlantic/South_Georgia",
		},
	},
	{
		Label: "Australia",
		Zones: []string{
			"Australia/Adelaide", "Australia/Brisbane", "Australia/Darwin",
			"Australia/Melbourne", "Australia/Perth", "Australia/Sydney",
		},
	},
	{
		Label: "Europe",
		Zones: []string{
			"Europe/Amsterdam", "Europe/Athens", "Europe/Berlin", "Europe/Brussels",
			"Europe/Bucharest", "Europe/Copenhagen", "Europe/Dublin", "Europe/Helsinki",
			"Europe/Lisbon", "Europe/London", "Europe/Madrid", "Europe/Moscow",
			"Europe/Oslo", "Europe/Paris", "Europe/Prague", "Europe/Rome",
			"Europe/Stockholm", "Europe/Vienna", "Europe/Warsaw", "Europe/Zurich",
		},
	},
	{
		Label: "Pacific",
		Zones: []string{
			"Pacific/Auckland", "Pacific/Fiji", "Pacific/Guam", "Pacific/Honolulu",
			"Pacific/Noumea", "Pacific/Port_Moresby", "Pacific/Tahiti",
		},
	},
}

// ValidTimezone reports whether name is a loadable IANA timezone.
func ValidTimezone(name string) bool {
	_, err := time.LoadLocation(name)
	return err == nil
}
