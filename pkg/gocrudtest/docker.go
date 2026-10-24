package gocrudtest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// Container is a PostgreSQL server in a docker container, which the tests use.
type Container struct {
	id string
}

// CreateDocker starts PostgreSQL in a docker container with the docker command, and returns the container and a
// connection to its database. It stops the tests when the database cannot be used.
func CreateDocker(user, pass, name string) (*Container, *sql.DB) {
	// the port is bound only on 127.0.0.1: docker can give the port a different number on IPv6, which would be the
	// port of another container on IPv4, e.g. of the tests of another package that run at the same time
	id, err := dockerOutput("run", "--detach", "--rm", "--publish", "127.0.0.1::5432",
		"--env", "POSTGRES_PASSWORD="+pass, "--env", "POSTGRES_USER="+user, "--env", "POSTGRES_DB="+name,
		"postgres:16-alpine")
	if err != nil {
		log.Fatalf("Could not start PostgreSQL in docker: %s", err)
	}
	container := &Container{id: id}

	address, err := dockerOutput("port", id, "5432/tcp")
	if err != nil {
		RemoveDocker(container)
		log.Fatalf("Could not get the port of PostgreSQL in docker: %s", err)
	}
	_, port, _ := strings.Cut(address, "127.0.0.1:")

	db, err := sql.Open("postgres", fmt.Sprintf("host=127.0.0.1 port=%s user=%s password=%s dbname=%s sslmode=disable", port, user, pass, name))
	if err != nil {
		RemoveDocker(container)
		log.Fatalf("Could not open the database in docker: %s", err)
	}

	// the server accepts connections when it has created the database, and restarted
	for deadline := time.Now().Add(time.Minute); ; time.Sleep(200 * time.Millisecond) {
		err = db.PingContext(context.Background())
		if err == nil {
			return container, db
		}
		if time.Now().After(deadline) {
			RemoveDocker(container)
			log.Fatalf("Could not connect to the database in docker: %s", err)
		}
	}
}

func RemoveDocker(container *Container) {
	if _, err := dockerOutput("rm", "--force", container.id); err != nil {
		log.Fatalf("Error removing docker container: %s", err)
	}
}

// dockerOutput runs the docker command with args, and returns its output without the line breaks around it.
func dockerOutput(args ...string) (string, error) {
	output, err := exec.CommandContext(context.Background(), "docker", args...).Output() // #nosec G204 -- docker, with the arguments of the tests, without a shell
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", fmt.Errorf("docker %s: %w: %s", args[0], err, strings.TrimSpace(string(exitErr.Stderr)))
	}
	return strings.TrimSpace(string(output)), err
}
