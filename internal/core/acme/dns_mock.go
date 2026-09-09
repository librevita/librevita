package acme

import (
	"context"
	"sync"
)

// MockDNSProvider is a thread-safe in-memory DNSProvider for testing.
type MockDNSProvider struct {
	mu      sync.Mutex
	records map[string]string // domain -> keyAuthRecord
}

// NewMockDNSProvider creates a new MockDNSProvider.
func NewMockDNSProvider() *MockDNSProvider {
	return &MockDNSProvider{
		records: make(map[string]string),
	}
}

func (m *MockDNSProvider) Present(ctx context.Context, domain, keyAuthRecord string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records[domain] = keyAuthRecord
	return nil
}

func (m *MockDNSProvider) CleanUp(ctx context.Context, domain, keyAuthRecord string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, domain)
	return nil
}

// GetRecord returns the stored record for a domain.
func (m *MockDNSProvider) GetRecord(domain string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.records[domain]
	return rec, ok
}
