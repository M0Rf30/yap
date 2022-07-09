package debian_test

import (
	"reflect"
	"testing"

	"github.com/packagefoundation/yap/debian"
)

func TestShieldVersions(t *testing.T) {
	t.Parallel()

	type args struct {
		packages []string
	}

	tests := []struct {
		name                 string
		args                 args
		wantShieldedPackages []string
	}{
		{
			name: "no version",
			args: args{
				packages: []string{"dep1", "dep2", "dep3"},
			},
			wantShieldedPackages: []string{"dep1", "dep2", "dep3"},
		},
		{
			name: "mixed",
			args: args{
				packages: []string{"dep1 >=0.1", "dep2< 3", "dep3  >  9", "dep4 <=  3.1.2.3", "dep5"},
			},
			wantShieldedPackages: []string{"dep1 (>=0.1)", "dep2 (<3)", "dep3 (>9)", "dep4 (<=3.1.2.3)", "dep5"},
		},
	}

	for _, tt := range tests { // nolint:paralleltest
		t.Run(tt.name, func(t *testing.T) {
			if gotShieldedPackages := debian.ShieldVersions(tt.args.packages); !reflect.DeepEqual(gotShieldedPackages,
				tt.wantShieldedPackages) {
				t.Parallel()
				t.Errorf("ShieldVersions() = %v, want %v", gotShieldedPackages, tt.wantShieldedPackages)
			}
		})
	}
}
