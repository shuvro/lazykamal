package gui

import (
	"sync"
	"time"
)

// Spinner provides an animated loading indicator
type Spinner struct {
	frames   []string
	current  int
	message  string
	running  bool
	mu       sync.Mutex
	stopCh   chan struct{}
	updateFn func()
}

// NewSpinner creates a new spinner with a message
func NewSpinner(message string, updateFn func()) *Spinner {
	return &Spinner{
		frames:   spinnerFrames,
		message:  message,
		updateFn: updateFn,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.mu.Lock()
				s.current = (s.current + 1) % len(s.frames)
				s.mu.Unlock()
				if s.updateFn != nil {
					s.updateFn()
				}
			}
		}
	}()
}

// Stop stops the spinner animation
func (s *Spinner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopCh)
}

// Frame returns the current spinner frame
func (s *Spinner) Frame() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return yellow(s.frames[s.current])
}

// String returns the spinner with its message
func (s *Spinner) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return ""
	}
	return yellow(s.frames[s.current]) + " " + s.message
}

// IsRunning returns whether the spinner is active
func (s *Spinner) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// SetMessage updates the spinner message
func (s *Spinner) SetMessage(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = msg
}
