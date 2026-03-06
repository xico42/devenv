package do

import (
	"context"
	"fmt"

	"github.com/digitalocean/godo"
)

// ListSSHKeys returns all SSH keys on the account.
func (c *Client) ListSSHKeys(ctx context.Context) ([]godo.Key, error) {
	keys, _, err := c.Keys.List(ctx, &godo.ListOptions{PerPage: 200})
	if err != nil {
		return nil, fmt.Errorf("listing SSH keys: %w", err)
	}
	return keys, nil
}

// ListRegions returns all available regions.
func (c *Client) ListRegions(ctx context.Context) ([]godo.Region, error) {
	regions, _, err := c.Regions.List(ctx, &godo.ListOptions{PerPage: 200})
	if err != nil {
		return nil, fmt.Errorf("listing regions: %w", err)
	}
	return regions, nil
}
