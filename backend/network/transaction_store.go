package network

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TransactionStore interface for storing and retrieving transactions (Single Responsibility)
type TransactionStore interface {
	Add(transaction HTTPTransaction) error
	Update(transaction HTTPTransaction) error
	GetAll() []HTTPTransaction
	GetByID(id string) (*HTTPTransaction, error)
	GetSummaries() []HTTPTransactionSummary
	GetSummariesAfter(lastID string) []HTTPTransactionSummary
	GetNextSequenceNumber() int
}

// InMemoryTransactionStore implements the TransactionStore interface
type InMemoryTransactionStore struct {
	transactions   []HTTPTransaction
	mutex          sync.RWMutex
	requestCounter int
}

// NewInMemoryTransactionStore creates a new in-memory transaction store
func NewInMemoryTransactionStore() TransactionStore {
	return &InMemoryTransactionStore{
		transactions: make([]HTTPTransaction, 0),
	}
}

// Add adds a new transaction to the store
func (s *InMemoryTransactionStore) Add(transaction HTTPTransaction) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Ensure transaction has an ID
	if transaction.ID == "" {
		transaction.ID = uuid.New().String()
	}

	// Ensure transaction has a timestamp
	if transaction.Timestamp == "" {
		transaction.Timestamp = time.Now().Format(time.RFC3339)
	}

	// Assign sequence number only if not already set
	if transaction.SeqNumber == 0 {
		s.requestCounter++
		transaction.SeqNumber = s.requestCounter
	} else {
		// If a sequence number is provided, ensure our counter stays synchronized
		// by updating it to the highest value we've seen
		if transaction.SeqNumber > s.requestCounter {
			s.requestCounter = transaction.SeqNumber
		}
	}

	s.transactions = append(s.transactions, transaction)
	return nil
}

// Update updates an existing transaction in the store
func (s *InMemoryTransactionStore) Update(transaction HTTPTransaction) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Find and update the transaction
	for i, tx := range s.transactions {
		if tx.ID == transaction.ID {
			// Preserve original sequence number and timestamp if not provided
			if transaction.SeqNumber == 0 {
				transaction.SeqNumber = tx.SeqNumber
			}
			if transaction.Timestamp == "" {
				transaction.Timestamp = tx.Timestamp
			}
			s.transactions[i] = transaction
			return nil
		}
	}

	return fmt.Errorf("transaction not found: %s", transaction.ID)
}

// GetAll returns all transactions
func (s *InMemoryTransactionStore) GetAll() []HTTPTransaction {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make([]HTTPTransaction, len(s.transactions))
	copy(result, s.transactions)
	return result
}

// GetByID returns a transaction by ID
func (s *InMemoryTransactionStore) GetByID(id string) (*HTTPTransaction, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	for _, tx := range s.transactions {
		if tx.ID == id {
			// Return a copy
			return &tx, nil
		}
	}

	return nil, fmt.Errorf("transaction not found: %s", id)
}

// GetSummaries returns summaries of all transactions
func (s *InMemoryTransactionStore) GetSummaries() []HTTPTransactionSummary {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	summaries := make([]HTTPTransactionSummary, len(s.transactions))
	for i, tx := range s.transactions {
		summary := HTTPTransactionSummary{
			ID:        tx.ID,
			Timestamp: tx.Timestamp,
			Method:    tx.Request.Method(),
			URL:       tx.Request.URL(),
			SeqNumber: tx.SeqNumber,
		}
		if tx.Response != nil {
			statusCode := tx.Response.StatusCode()
			summary.StatusCode = &statusCode
			status := tx.Response.Status()
			summary.Status = &status
			summary.ResponseSize = len(tx.Response.Dump)
		}
		summaries[i] = summary
	}
	return summaries
}

// GetSummariesAfter returns summaries of transactions after the given ID
func (s *InMemoryTransactionStore) GetSummariesAfter(lastID string) []HTTPTransactionSummary {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	startIdx := 0
	if lastID != "" {
		// Find the index of the last ID
		for i := len(s.transactions) - 1; i >= 0; i-- {
			if s.transactions[i].ID == lastID {
				startIdx = i + 1
				break
			}
		}

		// If lastID not found, return empty
		if startIdx == 0 {
			return []HTTPTransactionSummary{}
		}
	}

	if startIdx >= len(s.transactions) {
		return []HTTPTransactionSummary{}
	}

	newTransactions := s.transactions[startIdx:]
	summaries := make([]HTTPTransactionSummary, len(newTransactions))
	for i, tx := range newTransactions {
		summary := HTTPTransactionSummary{
			ID:        tx.ID,
			Timestamp: tx.Timestamp,
			Method:    tx.Request.Method(),
			URL:       tx.Request.URL(),
			SeqNumber: tx.SeqNumber,
		}
		if tx.Response != nil {
			statusCode := tx.Response.StatusCode()
			summary.StatusCode = &statusCode
			status := tx.Response.Status()
			summary.Status = &status
			summary.ResponseSize = len(tx.Response.Dump)
		}
		summaries[i] = summary
	}
	return summaries
}

// GetNextSequenceNumber returns the next sequence number
func (s *InMemoryTransactionStore) GetNextSequenceNumber() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.requestCounter++
	return s.requestCounter
}

// Reset resets the request counter
func (s *InMemoryTransactionStore) Reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.requestCounter = 0
	s.transactions = make([]HTTPTransaction, 0)
}

// SetCounter sets the request counter to a specific value
func (s *InMemoryTransactionStore) SetCounter(value int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.requestCounter = value
}
