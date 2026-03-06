package do_test

import (
	"context"
	"errors"
	"testing"

	"github.com/digitalocean/godo"

	"github.com/xico42/devenv/internal/do"
)

type mockKeysService struct {
	keys []godo.Key
	err  error
}

func (m *mockKeysService) List(_ context.Context, _ *godo.ListOptions) ([]godo.Key, *godo.Response, error) {
	return m.keys, &godo.Response{}, m.err
}

type mockRegionsService struct {
	regions []godo.Region
	err     error
}

func (m *mockRegionsService) List(_ context.Context, _ *godo.ListOptions) ([]godo.Region, *godo.Response, error) {
	return m.regions, &godo.Response{}, m.err
}

func TestClient_ListSSHKeys(t *testing.T) {
	c := &do.Client{
		Keys: &mockKeysService{
			keys: []godo.Key{{ID: 1, Name: "MyKey", Fingerprint: "aa:bb"}},
		},
	}
	keys, err := c.ListSSHKeys(context.Background())
	if err != nil {
		t.Fatalf("ListSSHKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0].Name != "MyKey" {
		t.Errorf("ListSSHKeys() = %v, want 1 key named MyKey", keys)
	}
}

func TestClient_ListSSHKeys_Error(t *testing.T) {
	c := &do.Client{
		Keys: &mockKeysService{err: errors.New("api error")},
	}
	_, err := c.ListSSHKeys(context.Background())
	if err == nil {
		t.Fatal("ListSSHKeys() = nil, want error")
	}
}

func TestClient_ListRegions(t *testing.T) {
	c := &do.Client{
		Regions: &mockRegionsService{
			regions: []godo.Region{{Slug: "nyc3", Name: "New York 3", Available: true}},
		},
	}
	regions, err := c.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions() error = %v", err)
	}
	if len(regions) != 1 || regions[0].Slug != "nyc3" {
		t.Errorf("ListRegions() = %v, want 1 region nyc3", regions)
	}
}

func TestClient_ListRegions_Error(t *testing.T) {
	c := &do.Client{
		Regions: &mockRegionsService{err: errors.New("api error")},
	}
	_, err := c.ListRegions(context.Background())
	if err == nil {
		t.Fatal("ListRegions() = nil, want error")
	}
}
