package mainnet

// Iterator walks a closed ledger range [start, end], yielding transactions
// one at a time. Lazy: fetches the next ledger only when the previous one
// is exhausted. Usage:
//
//	for it.Next() { tx := it.Tx(); ... }
//	if it.Err() != nil { ... }
type Iterator struct {
	client  *Client
	start   int
	end     int
	curSeq  int
	ledger  []map[string]any
	offset  int
	err     error
	current map[string]any
}

// NewIterator constructs an iterator over ledgers in [start, end] inclusive.
func NewIterator(c *Client, start, end int) *Iterator {
	return &Iterator{client: c, start: start, end: end, curSeq: start - 1}
}

// Next advances to the next transaction. Returns false when the range is
// exhausted OR the client errored. Call Err() to check.
func (it *Iterator) Next() bool {
	for {
		if it.offset < len(it.ledger) {
			it.current = it.ledger[it.offset]
			it.offset++
			return true
		}
		it.curSeq++
		if it.curSeq > it.end {
			return false
		}
		txs, err := it.client.LedgerTransactions(it.curSeq)
		if err != nil {
			it.err = err
			return false
		}
		it.ledger = txs
		it.offset = 0
	}
}

// Tx returns the current transaction. Only valid after Next() returns true.
func (it *Iterator) Tx() map[string]any { return it.current }

// Err returns the first error observed during iteration, if any.
func (it *Iterator) Err() error { return it.err }
