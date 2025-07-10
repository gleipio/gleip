package backend

import (
	"Gleip/backend/network"
	"fmt"
	"sync"
	"time"
)

// DefaultInterceptQueue implements the InterceptQueue interface
type DefaultInterceptQueue struct {
	queue map[string]*network.HTTPTransaction
	mutex sync.RWMutex
}

// NewInterceptQueue creates a new intercept queue
func NewInterceptQueue() InterceptQueue {
	return &DefaultInterceptQueue{
		queue: make(map[string]*network.HTTPTransaction),
	}
}

// Add adds a transaction to the intercept queue
func (q *DefaultInterceptQueue) Add(transaction *network.HTTPTransaction) error {
	if err := q.validateTransaction(transaction); err != nil {
		return fmt.Errorf("invalid transaction: %w", err)
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	// Check if transaction already exists
	if _, exists := q.queue[transaction.ID]; exists {
		return fmt.Errorf("transaction already exists in queue: %s", transaction.ID)
	}

	q.queue[transaction.ID] = transaction
	return nil
}

// Remove removes a transaction from the intercept queue
func (q *DefaultInterceptQueue) Remove(id string) error {
	if id == "" {
		return fmt.Errorf("transaction ID cannot be empty")
	}

	q.mutex.Lock()
	defer q.mutex.Unlock()

	if _, exists := q.queue[id]; !exists {
		return fmt.Errorf("transaction not found: %s", id)
	}

	delete(q.queue, id)
	return nil
}

// Get retrieves a transaction from the intercept queue
func (q *DefaultInterceptQueue) Get(id string) (*network.HTTPTransaction, bool) {
	if id == "" {
		return nil, false
	}

	q.mutex.RLock()
	defer q.mutex.RUnlock()

	transaction, exists := q.queue[id]
	return transaction, exists
}

// GetAll returns all transactions in the intercept queue
func (q *DefaultInterceptQueue) GetAll() []*network.HTTPTransaction {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	transactions := make([]*network.HTTPTransaction, 0, len(q.queue))
	for _, tx := range q.queue {
		transactions = append(transactions, tx)
	}
	return transactions
}

// Clear removes all transactions from the intercept queue
func (q *DefaultInterceptQueue) Clear() {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// Close all done channels before clearing
	for _, tx := range q.queue {
		if tx.Done != nil {
			select {
			case <-tx.Done:
				// Already closed
			default:
				close(tx.Done)
			}
		}
	}

	q.queue = make(map[string]*network.HTTPTransaction)
}

// validateTransaction validates a transaction before adding it to the queue
func (q *DefaultInterceptQueue) validateTransaction(transaction *network.HTTPTransaction) error {
	if transaction == nil {
		return fmt.Errorf("transaction cannot be nil")
	}

	if transaction.ID == "" {
		return fmt.Errorf("transaction ID cannot be empty")
	}

	if transaction.Request.Method() == "" {
		return fmt.Errorf("transaction request method cannot be empty")
	}

	if transaction.Request.Host == "" {
		return fmt.Errorf("transaction request host cannot be empty")
	}

	if transaction.Done == nil {
		return fmt.Errorf("transaction Done channel cannot be nil")
	}

	return nil
}

// Count returns the number of transactions in the queue
func (q *DefaultInterceptQueue) Count() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	return len(q.queue)
}

// GetOldest returns the oldest transaction in the queue based on timestamp
func (q *DefaultInterceptQueue) GetOldest() (*network.HTTPTransaction, bool) {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	if len(q.queue) == 0 {
		return nil, false
	}

	var oldest *network.HTTPTransaction
	for _, tx := range q.queue {
		if oldest == nil || tx.Timestamp < oldest.Timestamp {
			oldest = tx
		}
	}

	return oldest, true
}

// RemoveOlderThan removes all transactions older than the specified duration
func (q *DefaultInterceptQueue) RemoveOlderThan(duration time.Duration) int {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	cutoffTime := time.Now().Add(-duration)
	removed := 0

	for id, tx := range q.queue {
		// Parse the timestamp
		txTime, err := time.Parse(time.RFC3339, tx.Timestamp)
		if err != nil {
			continue
		}

		if txTime.Before(cutoffTime) {
			// Close done channel if not already closed
			if tx.Done != nil {
				select {
				case <-tx.Done:
					// Already closed
				default:
					close(tx.Done)
				}
			}
			delete(q.queue, id)
			removed++
		}
	}

	return removed
}

// GetByStatus returns all transactions with a specific waiting status
func (q *DefaultInterceptQueue) GetByStatus(waitingForResponse bool) []*network.HTTPTransaction {
	q.mutex.RLock()
	defer q.mutex.RUnlock()

	transactions := make([]*network.HTTPTransaction, 0)
	for _, tx := range q.queue {
		if tx.WaitingForResponse == waitingForResponse {
			transactions = append(transactions, tx)
		}
	}
	return transactions
}
