package stage

import "strings"

const KilledDetail = "no output — the analysis was killed before it could report " +
	"(a large tree under memory pressure is the usual cause)"

func Absent(ok bool, findings int) bool {
	return !ok && findings == 0
}

func Note(name, detail string) string {
	if strings.TrimSpace(detail) == "" {
		detail = KilledDetail
	}
	return name + " DID NOT RUN — its result is absent, not clean:\n" + detail
}
