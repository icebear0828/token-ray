package source

import (
	"time"

	"ai-flight-dashboard/internal/calculator"
	"ai-flight-dashboard/internal/db"
	"ai-flight-dashboard/internal/model"
)

// DBSink is the host-side Sink implementation. It owns cost calculation,
// persistence, and change notification so sources never need to know about
// the database or pricing.
type DBSink struct {
	db       *db.DB
	calc     *calculator.Calculator
	deviceID string
	notify   func(model.TokenUsage)
}

func NewDBSink(database *db.DB, calc *calculator.Calculator, deviceID string, notify func(model.TokenUsage)) *DBSink {
	return &DBSink{db: database, calc: calc, deviceID: deviceID, notify: notify}
}

func (s *DBSink) DeviceID() string { return s.deviceID }

func (s *DBSink) Emit(u model.TokenUsage, filePath string) error {
	ts := u.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	u.Timestamp = ts
	cost, _ := s.calc.CalculateCost(u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens)
	if err := s.db.InsertUsageWithTime(u, cost, ts, filePath, s.deviceID); err != nil {
		return err
	}
	if s.notify != nil {
		s.notify(u)
	}
	return nil
}

func (s *DBSink) Offset(key string) (int64, error) {
	return s.db.GetOffset(key)
}

func (s *DBSink) SetOffset(key string, offset int64) error {
	return s.db.SetOffset(key, offset)
}

func (s *DBSink) Supersede(source, filePath, deviceID, uuidPrefix string) (int64, error) {
	return s.db.SupersedeUsageBySourceFilePathDeviceUUIDPrefix(source, filePath, deviceID, uuidPrefix)
}

// KnownUsageByFile returns the usage rows already stored for a file, keyed by
// UUID. Sources that re-read a transcript after an append can use it to avoid
// re-emitting unchanged historical records.
func (s *DBSink) KnownUsageByFile(filePath string) (map[string]model.TokenUsage, error) {
	return s.db.QueryUsageByFileUUIDs(filePath, s.deviceID)
}

func (s *DBSink) KnownFiles() ([]string, error) {
	return s.db.ListKnownFiles()
}

func (s *DBSink) AllOffsets() (map[string]int64, error) {
	return s.db.AllOffsets()
}

func (s *DBSink) UpsertKnownDir(dir string) error {
	return s.db.UpsertKnownDir(dir)
}

// RunInTx runs fn against a sink whose writes commit atomically. Emitted usage
// is buffered and only delivered to the change notifier after the commit, so
// subscribers never observe rolled-back rows.
func (s *DBSink) RunInTx(fn func(Sink) error) error {
	var pending []model.TokenUsage
	err := s.db.RunInTx(func(tx *db.Tx) error {
		return fn(&txSink{
			tx:       tx,
			calc:     s.calc,
			deviceID: s.deviceID,
			notify:   func(u model.TokenUsage) { pending = append(pending, u) },
		})
	})
	if err != nil {
		return err
	}
	if s.notify != nil {
		for _, u := range pending {
			s.notify(u)
		}
	}
	return nil
}

// txSink is the transactional counterpart of DBSink handed to TxRunner callers.
type txSink struct {
	tx       *db.Tx
	calc     *calculator.Calculator
	deviceID string
	notify   func(model.TokenUsage)
}

func (s *txSink) DeviceID() string { return s.deviceID }

func (s *txSink) Emit(u model.TokenUsage, filePath string) error {
	ts := u.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	u.Timestamp = ts
	cost, _ := s.calc.CalculateCost(u.Model, u.InputTokens, u.CachedTokens, u.CacheCreationTokens, u.OutputTokens)
	if err := s.tx.InsertUsageWithTime(u, cost, ts, filePath, s.deviceID); err != nil {
		return err
	}
	if s.notify != nil {
		s.notify(u)
	}
	return nil
}

func (s *txSink) Offset(key string) (int64, error) {
	return s.tx.GetOffset(key)
}

func (s *txSink) SetOffset(key string, offset int64) error {
	return s.tx.SetOffset(key, offset)
}

func (s *txSink) Supersede(source, filePath, deviceID, uuidPrefix string) (int64, error) {
	return s.tx.SupersedeUsageBySourceFilePathDeviceUUIDPrefix(source, filePath, deviceID, uuidPrefix)
}

func (s *txSink) KnownFiles() ([]string, error) {
	return s.tx.ListKnownFiles()
}

func (s *txSink) AllOffsets() (map[string]int64, error) {
	return s.tx.AllOffsets()
}

func (s *txSink) UpsertKnownDir(dir string) error {
	return s.tx.UpsertKnownDir(dir)
}

var (
	_ JSONLStore  = (*DBSink)(nil)
	_ OffsetIndex = (*DBSink)(nil)
	_ TxRunner    = (*DBSink)(nil)
	_ JSONLStore  = (*txSink)(nil)
)
