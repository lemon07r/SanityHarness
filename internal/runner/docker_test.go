package runner

import (
	"runtime"
	"testing"
)

func TestPlatformString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		os   string
		arch string
		want string
	}{
		{
			name: "full platform",
			os:   "linux",
			arch: "arm64",
			want: "linux/arm64",
		},
		{
			name: "missing os",
			os:   "",
			arch: "amd64",
			want: "unknown/amd64",
		},
		{
			name: "missing arch",
			os:   "linux",
			arch: "",
			want: "linux/unknown",
		},
		{
			name: "missing os and arch",
			os:   "",
			arch: "",
			want: "unknown/unknown",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := platformString(tc.os, tc.arch)
			if got != tc.want {
				t.Fatalf("platformString(%q, %q) = %q, want %q", tc.os, tc.arch, got, tc.want)
			}
		})
	}
}

func TestHostPlatformString(t *testing.T) {
	t.Parallel()

	want := "linux/" + runtime.GOARCH
	got := hostPlatformString()
	if got != want {
		t.Fatalf("hostPlatformString() = %q, want %q", got, want)
	}
}
