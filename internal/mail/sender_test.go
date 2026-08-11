package mail

import (
	"testing"

	mailclient "github.com/wneessen/go-mail"
)

func TestParseTLSPolicy(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  mailclient.TLSPolicy
	}{
		{name: "mandatory", value: "mandatory", want: mailclient.TLSMandatory},
		{name: "opportunistic", value: "opportunistic", want: mailclient.TLSOpportunistic},
		{name: "none", value: "none", want: mailclient.NoTLS},
		{name: "normalized", value: " MANDATORY ", want: mailclient.TLSMandatory},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseTLSPolicy(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("parseTLSPolicy(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
	if _, err := parseTLSPolicy("invalid"); err == nil {
		t.Fatal("expected invalid policy to fail")
	}
}
