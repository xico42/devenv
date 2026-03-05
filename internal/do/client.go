package do

import (
	"context"
	"fmt"

	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"
)

// DropletsService is the subset of godo.DropletsService used by devenv.
type DropletsService interface {
	Create(ctx context.Context, req *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error)
	Get(ctx context.Context, dropletID int) (*godo.Droplet, *godo.Response, error)
	Delete(ctx context.Context, dropletID int) (*godo.Response, error)
}

// Client wraps the DO API with only the methods devenv needs.
type Client struct {
	Droplets DropletsService
}

// New returns an authenticated Client for the given token.
func New(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	g := godo.NewClient(tc)
	return &Client{Droplets: g.Droplets}, nil
}
