// Package sse parses server-sent event streams.
package sse

import (
	"bufio"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Event types used to group parsed events. Events without an explicit type
// are grouped under EventTypeMessage.
const (
	EventTypeMessage = "message"
	EventTypeError   = "error"
	EventTypePing    = "ping"
)

// Event is a single parsed server-sent event.
type Event struct {
	Type  string
	Data  string
	ID    string
	Retry int
}

// Parser collects server-sent events grouped by event type. It is safe for
// concurrent use.
type Parser struct {
	events map[string][]Event
	mu     sync.RWMutex
}

// NewParser returns an empty Parser.
func NewParser() *Parser {
	return &Parser{
		events: make(map[string][]Event),
	}
}

// Parse reads events from reader until EOF and stores them by type. A
// trailing event without a terminating blank line is discarded.
func (p *Parser) Parse(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)

	var currentEvent Event
	var dataLines []string

	for scanner.Scan() {
		line := scanner.Text()

		// Empty line indicates end of event
		if line == "" {
			if len(dataLines) > 0 {
				currentEvent.Data = strings.Join(dataLines, "\n")
			}

			// Default to message type if no event type specified
			if currentEvent.Type == "" {
				currentEvent.Type = EventTypeMessage
			}

			// Store the event
			p.mu.Lock()
			p.events[currentEvent.Type] = append(p.events[currentEvent.Type], currentEvent)
			p.mu.Unlock()

			// Reset for next event
			currentEvent = Event{}
			dataLines = nil
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Parse field:value format
		if colonIndex := strings.Index(line, ":"); colonIndex != -1 {
			field := line[:colonIndex]
			value := line[colonIndex+1:]

			// Remove leading space from value if present
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}

			switch field {
			case "event":
				currentEvent.Type = value
			case "data":
				dataLines = append(dataLines, value)
			case "id":
				currentEvent.ID = value
			case "retry":
				if retryMs, err := strconv.Atoi(value); err == nil {
					currentEvent.Retry = retryMs
				}
			}
		}
	}

	return scanner.Err()
}

// EventsByType returns a copy of the parsed events of the given type, in
// stream order.
func (p *Parser) EventsByType(eventType string) []Event {
	p.mu.RLock()
	defer p.mu.RUnlock()

	events := p.events[eventType]
	result := make([]Event, len(events))
	copy(result, events)
	return result
}

// MessageEvents returns a copy of the parsed events of type EventTypeMessage.
func (p *Parser) MessageEvents() []Event {
	return p.EventsByType(EventTypeMessage)
}

// AllEvents returns a copy of all parsed events keyed by event type.
func (p *Parser) AllEvents() map[string][]Event {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string][]Event)
	for eventType, events := range p.events {
		eventsCopy := make([]Event, len(events))
		copy(eventsCopy, events)
		result[eventType] = eventsCopy
	}
	return result
}
