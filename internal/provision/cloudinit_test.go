package provision_test

import (
	"strings"
	"testing"

	"github.com/xico42/devenv/internal/provision"
)

func TestRenderUserData(t *testing.T) {
	tests := []struct {
		name     string
		params   provision.Params
		contains []string
		wantErr  bool
	}{
		{
			name: "renders all params",
			params: provision.Params{
				Username:         "devuser",
				Hostname:         "devenv-test",
				TailscaleAuthKey: "tskey-abc123",
			},
			contains: []string{
				"#cloud-config",
				"devuser",
				"devenv-test",
				"tskey-abc123",
			},
		},
		{
			name: "minimal params",
			params: provision.Params{
				Username: "devuser",
				Hostname: "devenv-minimal",
			},
			contains: []string{"#cloud-config", "devuser", "devenv-minimal"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := provision.RenderUserData(tt.params)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RenderUserData() error = %v, wantErr %v", err, tt.wantErr)
			}
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}
