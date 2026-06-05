package wiki

import "testing"

// rationaleGetter is the shared interface both BaseArgs and BaseWriteArgs
// satisfy via GetRationale. Handlers extract the rationale uniformly without a
// per-type switch; this test pins that contract.
type rationaleGetter interface {
	GetRationale() string
}

func TestGetRationale(t *testing.T) {
	tests := []struct {
		name string
		arg  rationaleGetter
		want string
	}{
		{
			name: "BaseArgs with rationale",
			arg:  BaseArgs{Rationale: "checking onboarding docs"},
			want: "checking onboarding docs",
		},
		{
			name: "BaseArgs empty (reads make rationale optional)",
			arg:  BaseArgs{},
			want: "",
		},
		{
			name: "BaseWriteArgs with rationale",
			arg:  BaseWriteArgs{Rationale: "striking former employee name"},
			want: "striking former employee name",
		},
		{
			name: "BaseWriteArgs empty",
			arg:  BaseWriteArgs{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.arg.GetRationale(); got != tt.want {
				t.Errorf("GetRationale() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestGetRationale_InterfaceSatisfaction asserts both arg types can be passed
// through the same GetRationale() interface, which is the whole point of the
// shared method (the audit logger sees one interface across reads and writes).
func TestGetRationale_InterfaceSatisfaction(t *testing.T) {
	var _ rationaleGetter = BaseArgs{}
	var _ rationaleGetter = BaseWriteArgs{}

	// Embedded use: a derived args struct inherits GetRationale through BaseArgs.
	search := SearchArgs{BaseArgs: BaseArgs{Rationale: "find deployment page"}}
	if got := search.GetRationale(); got != "find deployment page" {
		t.Errorf("embedded GetRationale() = %q, want %q", got, "find deployment page")
	}

	edit := EditPageArgs{BaseWriteArgs: BaseWriteArgs{Rationale: "publish release notes"}}
	if got := edit.GetRationale(); got != "publish release notes" {
		t.Errorf("embedded GetRationale() = %q, want %q", got, "publish release notes")
	}
}
