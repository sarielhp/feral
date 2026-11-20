package calendar

import (
	"strconv"
	"strings"
	"time"
)

// windowsToIANA maps Microsoft Windows time zone names (standard in Exchange/Outlook
// iCalendar attachments) to canonical IANA time zone identifiers according to
// Unicode CLDR.
var windowsToIANA = map[string]string{
	"Dateline Standard Time":          "Etc/GMT+12",
	"UTC-11":                          "Etc/GMT+11",
	"Aleutian Standard Time":          "America/Adak",
	"Hawaiian Standard Time":          "Pacific/Honolulu",
	"Marquesas Standard Time":         "Pacific/Marquesas",
	"Alaskan Standard Time":           "America/Anchorage",
	"Alaska Standard Time":            "America/Anchorage",
	"UTC-09":                          "Etc/GMT+9",
	"Pacific Standard Time (Mexico)":  "America/Tijuana",
	"UTC-08":                          "Etc/GMT+8",
	"Pacific Standard Time":           "America/Los_Angeles",
	"US Mountain Standard Time":       "America/Phoenix",
	"Mountain Standard Time (Mexico)": "America/Mazatlan",
	"Mountain Standard Time":          "America/Denver",
	"Yukon Standard Time":             "America/Whitehorse",
	"Central America Standard Time":   "America/Guatemala",
	"Central Standard Time":           "America/Chicago",
	"Easter Island Standard Time":     "Pacific/Easter",
	"Central Standard Time (Mexico)":  "America/Mexico_City",
	"Canada Central Standard Time":    "America/Regina",
	"SA Pacific Standard Time":        "America/Bogota",
	"Eastern Standard Time (Mexico)":  "America/Cancun",
	"Eastern Standard Time":           "America/New_York",
	"Haiti Standard Time":             "America/Port-au-Prince",
	"Cuba Standard Time":              "America/Havana",
	"US Eastern Standard Time":        "America/Indianapolis",
	"Turks And Caicos Standard Time":  "America/Grand_Turk",
	"Paraguay Standard Time":          "America/Asuncion",
	"Atlantic Standard Time":          "America/Halifax",
	"Venezuela Standard Time":         "America/Caracas",
	"Central Brazilian Standard Time": "America/Cuiaba",
	"SA Western Standard Time":        "America/La_Paz",
	"Pacific SA Standard Time":        "America/Santiago",
	"Newfoundland Standard Time":      "America/St_Johns",
	"Tocantins Standard Time":         "America/Araguaina",
	"E. South America Standard Time":  "America/Sao_Paulo",
	"SA Eastern Standard Time":        "America/Cayenne",
	"Argentina Standard Time":         "America/Buenos_Aires",
	"Greenland Standard Time":         "America/Godthab",
	"Montevideo Standard Time":        "America/Montevideo",
	"Magallanes Standard Time":        "America/Punta_Arenas",
	"Saint Pierre Standard Time":      "America/Miquelon",
	"Bahia Standard Time":             "America/Bahia",
	"UTC-02":                          "Etc/GMT+2",
	"Mid-Atlantic Standard Time":      "Etc/GMT+2",
	"Azores Standard Time":            "Atlantic/Azores",
	"Cape Verde Standard Time":        "Atlantic/Cape_Verde",
	"UTC":                             "UTC",
	"GMT Standard Time":               "Europe/London",
	"Greenwich Standard Time":         "Atlantic/Reykjavik",
	"Sao Tome Standard Time":          "Africa/Sao_Tome",
	"Morocco Standard Time":           "Africa/Casablanca",
	"W. Europe Standard Time":         "Europe/Berlin",
	"Central Europe Standard Time":    "Europe/Budapest",
	"Romance Standard Time":           "Europe/Paris",
	"Central European Standard Time":  "Europe/Warsaw",
	"W. Central Africa Standard Time": "Africa/Lagos",
	"Jordan Standard Time":            "Asia/Amman",
	"GTB Standard Time":               "Europe/Bucharest",
	"Middle East Standard Time":       "Asia/Beirut",
	"Egypt Standard Time":             "Africa/Cairo",
	"E. Europe Standard Time":         "Europe/Chisinau",
	"Syria Standard Time":             "Asia/Damascus",
	"West Bank Standard Time":         "Asia/Hebron",
	"South Africa Standard Time":      "Africa/Johannesburg",
	"FLE Standard Time":               "Europe/Kiev",
	"Israel Standard Time":            "Asia/Jerusalem",
	"South Sudan Standard Time":       "Africa/Juba",
	"Kaliningrad Standard Time":       "Europe/Kaliningrad",
	"Sudan Standard Time":             "Africa/Khartoum",
	"Libya Standard Time":             "Africa/Tripoli",
	"Namibia Standard Time":           "Africa/Windhoek",
	"Arabic Standard Time":            "Asia/Baghdad",
	"Turkey Standard Time":            "Europe/Istanbul",
	"Arab Standard Time":              "Asia/Riyadh",
	"Belarus Standard Time":           "Europe/Minsk",
	"Russian Standard Time":           "Europe/Moscow",
	"E. Africa Standard Time":         "Africa/Nairobi",
	"Volgograd Standard Time":         "Europe/Volgograd",
	"Iran Standard Time":              "Asia/Tehran",
	"Arabian Standard Time":           "Asia/Dubai",
	"Astrakhan Standard Time":         "Europe/Astrakhan",
	"Azerbaijan Standard Time":        "Asia/Baku",
	"Russia Time Zone 3":              "Europe/Samara",
	"Mauritius Standard Time":         "Indian/Mauritius",
	"Saratov Standard Time":           "Europe/Saratov",
	"Georgian Standard Time":          "Asia/Tbilisi",
	"Caucasus Standard Time":          "Asia/Yerevan",
	"Afghanistan Standard Time":       "Asia/Kabul",
	"West Asia Standard Time":         "Asia/Tashkent",
	"Ekaterinburg Standard Time":      "Asia/Yekaterinburg",
	"Pakistan Standard Time":          "Asia/Karachi",
	"Qyzylorda Standard Time":         "Asia/Qyzylorda",
	"India Standard Time":             "Asia/Kolkata",
	"Sri Lanka Standard Time":         "Asia/Colombo",
	"Nepal Standard Time":             "Asia/Kathmandu",
	"Central Asia Standard Time":      "Asia/Almaty",
	"Bangladesh Standard Time":        "Asia/Dhaka",
	"Omsk Standard Time":              "Asia/Omsk",
	"Myanmar Standard Time":           "Asia/Yangon",
	"SE Asia Standard Time":           "Asia/Bangkok",
	"Altai Standard Time":             "Asia/Barnaul",
	"W. Mongolia Standard Time":       "Asia/Hovd",
	"North Asia Standard Time":        "Asia/Krasnoyarsk",
	"N. Central Asia Standard Time":   "Asia/Novosibirsk",
	"Tomsk Standard Time":             "Asia/Tomsk",
	"China Standard Time":             "Asia/Shanghai",
	"North Asia East Standard Time":   "Asia/Irkutsk",
	"Singapore Standard Time":         "Asia/Singapore",
	"W. Australia Standard Time":      "Australia/Perth",
	"Taipei Standard Time":            "Asia/Taipei",
	"Ulaanbaatar Standard Time":       "Asia/Ulaanbaatar",
	"Aus Central W. Standard Time":    "Australia/Eucla",
	"Transbaikal Standard Time":       "Asia/Chita",
	"Tokyo Standard Time":             "Asia/Tokyo",
	"North Korea Standard Time":       "Asia/Pyongyang",
	"Korea Standard Time":             "Asia/Seoul",
	"Yakutsk Standard Time":           "Asia/Yakutsk",
	"Cen. Australia Standard Time":    "Australia/Adelaide",
	"AUS Central Standard Time":       "Australia/Darwin",
	"E. Australia Standard Time":      "Australia/Brisbane",
	"AUS Eastern Standard Time":       "Australia/Sydney",
	"West Pacific Standard Time":      "Pacific/Port_Moresby",
	"Tasmania Standard Time":          "Australia/Hobart",
	"Vladivostok Standard Time":       "Asia/Vladivostok",
	"Lord Howe Standard Time":         "Australia/Lord_Howe",
	"Bougainville Standard Time":      "Pacific/Bougainville",
	"Russia Time Zone 10":             "Asia/Srednekolymsk",
	"Magadan Standard Time":           "Asia/Magadan",
	"Norfolk Standard Time":           "Pacific/Norfolk",
	"Sakhalin Standard Time":          "Asia/Sakhalin",
	"Central Pacific Standard Time":   "Pacific/Guadalcanal",
	"Russia Time Zone 11":             "Asia/Kamchatka",
	"New Zealand Standard Time":       "Pacific/Auckland",
	"UTC+12":                          "Etc/GMT-12",
	"Fiji Standard Time":              "Pacific/Fiji",
	"Chatham Islands Standard Time":   "Pacific/Chatham",
	"UTC+13":                          "Etc/GMT-13",
	"Tonga Standard Time":             "Pacific/Tongatapu",
	"Samoa Standard Time":             "Pacific/Apia",
	"Line Islands Standard Time":      "Pacific/Kiritimati",
}

var tzAliases = map[string]string{
	"est":                         "America/New_York",
	"edt":                         "America/New_York",
	"eastern":                     "America/New_York",
	"eastern time":                "America/New_York",
	"eastern time (us & canada)":  "America/New_York",
	"eastern daylight time":       "America/New_York",
	"cst":                         "America/Chicago",
	"cdt":                         "America/Chicago",
	"central":                     "America/Chicago",
	"central time":                "America/Chicago",
	"central time (us & canada)":  "America/Chicago",
	"central daylight time":       "America/Chicago",
	"mst":                         "America/Denver",
	"mdt":                         "America/Denver",
	"mountain":                    "America/Denver",
	"mountain time":               "America/Denver",
	"mountain time (us & canada)": "America/Denver",
	"mountain daylight time":      "America/Denver",
	"pst":                         "America/Los_Angeles",
	"pdt":                         "America/Los_Angeles",
	"pacific":                     "America/Los_Angeles",
	"pacific time":                "America/Los_Angeles",
	"pacific time (us & canada)":  "America/Los_Angeles",
	"pacific daylight time":       "America/Los_Angeles",
	"akst":                        "America/Anchorage",
	"akdt":                        "America/Anchorage",
	"hst":                         "Pacific/Honolulu",
	"bst":                         "Europe/London",
	"gmt":                         "Etc/GMT",
	"utc":                         "UTC",
	"cet":                         "Europe/Paris",
	"cest":                        "Europe/Paris",
	"eet":                         "Europe/Athens",
	"eest":                        "Europe/Athens",
	"wet":                         "Europe/Lisbon",
	"west":                        "Europe/Lisbon",
	"ist":                         "Asia/Kolkata",
	"jst":                         "Asia/Tokyo",
	"kst":                         "Asia/Seoul",
	"aest":                        "Australia/Sydney",
	"aedt":                        "Australia/Sydney",
	"acst":                        "Australia/Adelaide",
	"acdt":                        "Australia/Adelaide",
	"awst":                        "Australia/Perth",
	"nzst":                        "Pacific/Auckland",
	"nzdt":                        "Pacific/Auckland",
	"idt":                         "Asia/Jerusalem",
	"msk":                         "Europe/Moscow",
}

// CleanTZID trims quotes, prefixes, and formatting artifacts from a raw TZID.
func CleanTZID(tz string) string {
	tz = strings.TrimSpace(strings.Trim(tz, "\"'"))
	tz = strings.TrimPrefix(tz, "/")
	if idx := strings.Index(tz, "zoneinfo/"); idx != -1 {
		tz = tz[idx+len("zoneinfo/"):]
	}
	tz = strings.TrimPrefix(tz, "tzone://Microsoft/")
	if strings.HasPrefix(tz, "(") {
		if endIdx := strings.Index(tz, ")"); endIdx != -1 {
			prefix := tz[1:endIdx]
			if strings.HasPrefix(prefix, "UTC") || strings.HasPrefix(prefix, "GMT") {
				tz = strings.TrimSpace(tz[endIdx+1:])
			}
		}
	}
	return tz
}

// ResolveTimeZone attempts to map an arbitrary TZID (including Windows time zone
// names, abbreviations, and path-prefixed names) to a canonical IANA time zone.
// Returns an empty string if unresolvable.
func ResolveTimeZone(tz string) string {
	cleaned := CleanTZID(tz)
	if cleaned == "" {
		return ""
	}

	// 1. Check exact match in Windows to IANA map.
	if iana, ok := windowsToIANA[cleaned]; ok {
		if _, err := time.LoadLocation(iana); err == nil {
			return iana
		}
	}

	// 2. Case-insensitive lookup in aliases (e.g. EST -> America/New_York).
	lower := strings.ToLower(cleaned)
	if iana, ok := tzAliases[lower]; ok {
		if _, err := time.LoadLocation(iana); err == nil {
			return iana
		}
	}

	// 3. Case-insensitive lookup in Windows map.
	for winName, iana := range windowsToIANA {
		if strings.EqualFold(winName, cleaned) {
			if _, err := time.LoadLocation(iana); err == nil {
				return iana
			}
		}
	}

	// 4. Check if the string directly loads as a valid IANA location.
	if _, err := time.LoadLocation(cleaned); err == nil {
		return cleaned
	}

	// 5. If path contains slashes (e.g. "/mozilla.org/.../America/New_York"), check tail.
	if parts := strings.Split(cleaned, "/"); len(parts) >= 2 {
		tail := parts[len(parts)-2] + "/" + parts[len(parts)-1]
		if _, err := time.LoadLocation(tail); err == nil {
			return tail
		}
	}

	return ""
}

// ParseUTCOffset parses an offset string such as "+0200", "-05:00", "UTC+0300"
// into seconds relative to UTC.
func ParseUTCOffset(val string) (int, bool) {
	val = strings.TrimSpace(val)
	val = strings.TrimPrefix(val, "UTC")
	val = strings.TrimPrefix(val, "GMT")
	val = strings.TrimSpace(val)
	if len(val) < 4 {
		return 0, false
	}

	sign := 1
	if val[0] == '-' {
		sign = -1
		val = val[1:]
	} else if val[0] == '+' {
		val = val[1:]
	}

	val = strings.ReplaceAll(val, ":", "")
	if len(val) < 4 {
		return 0, false
	}

	hours, err1 := strconv.Atoi(val[:2])
	mins, err2 := strconv.Atoi(val[2:4])
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return sign * (hours*3600 + mins*60), true
}
