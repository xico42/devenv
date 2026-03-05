package do_test

import (
	"context"
	"errors"
	"testing"

	"github.com/digitalocean/godo"

	"github.com/xico42/devenv/internal/do"
)

// mockDroplets implements DropletsService for testing.
type mockDroplets struct {
	createFn func(ctx context.Context, req *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error)
	getFn    func(ctx context.Context, id int) (*godo.Droplet, *godo.Response, error)
	deleteFn func(ctx context.Context, id int) (*godo.Response, error)
}

func (m *mockDroplets) Create(ctx context.Context, req *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error) {
	return m.createFn(ctx, req)
}
func (m *mockDroplets) Get(ctx context.Context, id int) (*godo.Droplet, *godo.Response, error) {
	return m.getFn(ctx, id)
}
func (m *mockDroplets) Delete(ctx context.Context, id int) (*godo.Response, error) {
	return m.deleteFn(ctx, id)
}

func TestCreateDroplet(t *testing.T) {
	tests := []struct {
		name    string
		mock    *mockDroplets
		req     do.CreateDropletRequest
		wantID  int
		wantErr bool
	}{
		{
			name: "success",
			mock: &mockDroplets{
				createFn: func(_ context.Context, req *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error) {
					return &godo.Droplet{ID: 42, Name: req.Name}, nil, nil
				},
			},
			req:    do.CreateDropletRequest{Name: "devenv-test", Region: "nyc3", Size: "s-2vcpu-4gb", Image: "ubuntu-24-04-x64"},
			wantID: 42,
		},
		{
			name: "api error",
			mock: &mockDroplets{
				createFn: func(_ context.Context, _ *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error) {
					return nil, nil, errors.New("api error")
				},
			},
			req:     do.CreateDropletRequest{Name: "fail"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &do.Client{Droplets: tt.mock}
			got, err := c.CreateDroplet(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateDroplet() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.ID != tt.wantID {
				t.Errorf("Droplet.ID = %d, want %d", got.ID, tt.wantID)
			}
		})
	}
}

func TestGetDroplet(t *testing.T) {
	tests := []struct {
		name    string
		id      int
		fn      func(context.Context, int) (*godo.Droplet, *godo.Response, error)
		wantErr bool
	}{
		{
			name: "success",
			id:   42,
			fn: func(_ context.Context, id int) (*godo.Droplet, *godo.Response, error) {
				return &godo.Droplet{ID: id}, nil, nil
			},
		},
		{
			name: "not found",
			id:   99,
			fn: func(_ context.Context, _ int) (*godo.Droplet, *godo.Response, error) {
				return nil, nil, errors.New("not found")
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &do.Client{Droplets: &mockDroplets{getFn: tt.fn}}
			_, err := c.GetDroplet(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDroplet() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeleteDroplet(t *testing.T) {
	tests := []struct {
		name    string
		fn      func(context.Context, int) (*godo.Response, error)
		wantErr bool
	}{
		{
			name: "success",
			fn:   func(_ context.Context, _ int) (*godo.Response, error) { return nil, nil },
		},
		{
			name:    "error",
			fn:      func(_ context.Context, _ int) (*godo.Response, error) { return nil, errors.New("delete failed") },
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &do.Client{Droplets: &mockDroplets{deleteFn: tt.fn}}
			err := c.DeleteDroplet(context.Background(), 42)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteDroplet() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
