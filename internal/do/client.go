package do

import (
	"context"
	"fmt"

	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"
)

// DropletsService is the subset of godo.DropletsService used by codeherd.
type DropletsService interface {
	Create(ctx context.Context, req *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error)
	Get(ctx context.Context, dropletID int) (*godo.Droplet, *godo.Response, error)
	Delete(ctx context.Context, dropletID int) (*godo.Response, error)
}

// SSHKeysService is the subset of godo.KeysService used by codeherd.
type SSHKeysService interface {
	List(ctx context.Context, opts *godo.ListOptions) ([]godo.Key, *godo.Response, error)
}

// RegionsService is the subset of godo.RegionsService used by codeherd.
type RegionsService interface {
	List(ctx context.Context, opts *godo.ListOptions) ([]godo.Region, *godo.Response, error)
}

// Client wraps the DO API with only the methods codeherd needs.
type Client struct {
	Droplets DropletsService
	Keys     SSHKeysService
	Regions  RegionsService
}

// New returns an authenticated Client for the given token.
func New(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	g := godo.NewClient(tc)
	return &Client{
		Droplets: g.Droplets,
		Keys:     g.Keys,
		Regions:  g.Regions,
	}, nil
}
