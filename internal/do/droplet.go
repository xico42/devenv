package do

import (
	"context"
	"fmt"

	"github.com/digitalocean/godo"
)

// CreateDropletRequest holds parameters for creating a droplet.
type CreateDropletRequest struct {
	Name     string
	Region   string
	Size     string
	Image    string
	SSHKeyID int
	UserData string
	Tags     []string
}

// CreateDroplet creates a new droplet and returns it.
func (c *Client) CreateDroplet(ctx context.Context, req CreateDropletRequest) (*godo.Droplet, error) {
	d, _, err := c.Droplets.Create(ctx, &godo.DropletCreateRequest{
		Name:     req.Name,
		Region:   req.Region,
		Size:     req.Size,
		Image:    godo.DropletCreateImage{Slug: req.Image},
		SSHKeys:  []godo.DropletCreateSSHKey{{ID: req.SSHKeyID}},
		UserData: req.UserData,
		Tags:     req.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("creating droplet: %w", err)
	}
	return d, nil
}

// GetDroplet returns the droplet with the given ID.
func (c *Client) GetDroplet(ctx context.Context, id int) (*godo.Droplet, error) {
	d, _, err := c.Droplets.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting droplet %d: %w", id, err)
	}
	return d, nil
}

// DeleteDroplet deletes the droplet with the given ID.
func (c *Client) DeleteDroplet(ctx context.Context, id int) error {
	if _, err := c.Droplets.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting droplet %d: %w", id, err)
	}
	return nil
}
