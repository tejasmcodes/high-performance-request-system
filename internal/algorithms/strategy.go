package algorithms

import "github.com/tejasmcodes/high-performance-request-system/internal/models"

type Strategy interface {
	NextServer([]*models.Server) *models.Server
}
