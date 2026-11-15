package web

import (
	"sync"

	"token-ray/internal/lan"
	"token-ray/internal/model"
)

// LANController is the dashboard-facing control surface for LAN discovery.
type LANController interface {
	CurrentLAN() *lan.LAN
	Status() model.LANStatusResponse
	Join() (model.LANStatusResponse, error)
	Leave() (model.LANStatusResponse, error)
}

type staticLANController struct {
	mu      sync.RWMutex
	lanInst *lan.LAN
	enabled bool
}

func newStaticLANController(lanInst *lan.LAN) LANController {
	return &staticLANController{
		lanInst: lanInst,
		enabled: lanInst != nil,
	}
}

func (c *staticLANController) CurrentLAN() *lan.LAN {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.enabled {
		return nil
	}
	return c.lanInst
}

func (c *staticLANController) Status() model.LANStatusResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return model.LANStatusResponse{Enabled: c.enabled && c.lanInst != nil}
}

func (c *staticLANController) Join() (model.LANStatusResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = c.lanInst != nil
	if c.enabled {
		c.lanInst.Ping()
	}
	return model.LANStatusResponse{Enabled: c.enabled}, nil
}

func (c *staticLANController) Leave() (model.LANStatusResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.enabled = false
	return model.LANStatusResponse{Enabled: false}, nil
}
