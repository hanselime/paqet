package socks

import (
	"context"
	"paqet/internal/client"
)

type Handler struct {
	client *client.Client
	ctx    context.Context
}
