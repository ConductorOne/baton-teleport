package main

import (
	cfg "github.com/conductorone/baton-teleport/pkg/config"
	"github.com/conductorone/baton-sdk/pkg/config"
)

func main() {
	config.Generate("teleport", cfg.Config)
}
