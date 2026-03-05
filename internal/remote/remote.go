package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client executes commands on a remote host over SSH.
type Client interface {
	Run(ctx context.Context, cmd string) (stdout, stderr string, err error)
	RunStream(ctx context.Context, cmd string, stdout io.Writer) error
	Close() error
}

type sshClient struct {
	conn *ssh.Client
}

// Dial opens an SSH connection to host:22 using the private key at identityFile.
// The connection is closed automatically when ctx is cancelled.
func Dial(ctx context.Context, host, user, identityFile string) (Client, error) {
	key, err := os.ReadFile(identityFile)
	if err != nil {
		return nil, fmt.Errorf("reading identity file: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // known_hosts support is a future task
		Timeout:         30 * time.Second,
	}
	conn, err := ssh.Dial("tcp", host+":22", cfg)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", host, err)
	}
	go func() {
		<-ctx.Done()
		conn.Close() //nolint:errcheck
	}()
	return &sshClient{conn: conn}, nil
}

func (c *sshClient) Run(ctx context.Context, cmd string) (string, string, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("new session: %w", err)
	}
	defer sess.Close() //nolint:errcheck

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	if err := sess.Run(cmd); err != nil {
		return stdout.String(), stderr.String(), fmt.Errorf("running %q: %w", cmd, err)
	}
	return stdout.String(), stderr.String(), nil
}

func (c *sshClient) RunStream(ctx context.Context, cmd string, stdout io.Writer) error {
	sess, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	defer sess.Close() //nolint:errcheck
	sess.Stdout = stdout
	if err := sess.Run(cmd); err != nil {
		return fmt.Errorf("running %q: %w", cmd, err)
	}
	return nil
}

func (c *sshClient) Close() error {
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("closing ssh connection: %w", err)
	}
	return nil
}
