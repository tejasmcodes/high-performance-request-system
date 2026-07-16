package models

type Server struct {
	ID                string
	URL               string
	Weight            int
	Healthy           bool
	ActiveConnections int
	ResponseTime      float64
}
