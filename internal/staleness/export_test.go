package staleness

//nolint:gochecknoglobals // export-for-test aliases so external _test pkg can reach unexported helpers
var (
	NormalizeMajorForTest = normalizeMajor
	ToleranceForTest      = tolerance
)
