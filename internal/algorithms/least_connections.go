package algorithms

import "github.com/tejasmcodes/high-performance-request-system/internal/models"

type LeastConnections struct{}

func (lc *LeastConnections) NextServer(servers []*models.Server) *models.Server {
	var selected *models.Server

	for _, server := range servers {
		if !server.Healthy {
			continue
		}

		if selected == nil{
			selected = server
		} else if server.ActiveConnections() < selected.ActiveConnections(){
			selected = server
		}
	}

	return selected
}