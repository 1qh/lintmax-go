//nolint:testpackage // the discriminator is unexported and is what these cases pin
package dupconst

import (
	"slices"
	"testing"
)

const (
	tArgCollectionID = "ArgCollectionID"
	tArgShareCollect = "argShareCollectID"
	tSrcPrincipals   = "srcPrincipals"
	tSrcPrincTable   = "srcPrincipalsTable"
	tXArgJSON        = "xArgJSON"
	tXJSON           = "xJSON"
	tDimColMonth     = "dimColMonth"
	tDimMonth        = "dimMonth"
	tPlausibleValue  = "plausibleValueMaxUSD"
	tPlausibleWeight = "plausibleWeightKgMax"
	tShareTitle      = "shareTitle"
	tEnrichTitle     = "enrichTitle"
	tAdminTitle      = "adminTitle"
	tMetricTitle     = "metricTitle"
	tokArg           = "arg"
)

func TestTokens(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		tArgShareCollect: {tokArg, "share", "collect", "id"},
		tArgCollectionID: {tokArg, "collection", "id"},
		tXArgJSON:        {"x", tokArg, "json"},
		tPlausibleValue:  {"plausible", "value", "max", "usd"},
	}
	for in, want := range cases {
		if got := tokens(in); !slices.Equal(got, want) {
			t.Errorf("tokens(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSameSubjectRejectsUnrelatedSubjects(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{
		{tPlausibleValue, tPlausibleWeight},
		{tShareTitle, tEnrichTitle},
		{tAdminTitle, tMetricTitle},
	}
	for _, p := range pairs {
		if sameSubject(p[0], p[1]) {
			t.Errorf("%q and %q name different subjects and must not group", p[0], p[1])
		}
	}
}

func TestSameSubjectAcceptsOneNamingTheOther(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{
		{tSrcPrincipals, tSrcPrincTable},
		{tXArgJSON, tXJSON},
		{tDimColMonth, tDimMonth},
		{tArgCollectionID, tArgShareCollect},
	}
	for _, p := range pairs {
		if !sameSubject(p[0], p[1]) {
			t.Errorf("%q and %q name one subject and must group", p[0], p[1])
		}
	}
}

func TestByDomainSplitsUnrelatedNames(t *testing.T) {
	t.Parallel()
	got := byDomain([]string{tShareTitle, tEnrichTitle, tAdminTitle, tMetricTitle})
	if len(got) != 0 {
		t.Fatalf("four unrelated subjects sharing a value must report nothing, got %v", got)
	}
}

func TestByDomainKeepsOneSubject(t *testing.T) {
	t.Parallel()
	got := byDomain([]string{tSrcPrincipals, tSrcPrincTable, tEnrichTitle})
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("want the two same-subject names as one group, got %v", got)
	}
}
