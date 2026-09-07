package models

import "sync"

type Server struct {
	ID                string
	URL               string
	Weight            int
	Healthy           bool
	activeConnections int
	ResponseTime      float64

	mu sync.Mutex
}

func(s *Server) ActiveConnections() int{
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.activeConnections

}

func(s *Server) IncrementConnections(){
	s.mu.Lock()
	defer s.mu.Unlock()

	s.activeConnections++
}


func(s *Server) DecrementConnections(){
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeConnections > 0{
		s.activeConnections--
	}
	
}
