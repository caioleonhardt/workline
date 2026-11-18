package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"workline/internal/domain"
)

// LatestScopeAck returns the activity's most recent ack (highest id), or nil.
func (s *Store) LatestScopeAck(ctx context.Context, activityID int64) (*domain.ScopeAck, error) {
	return latestScopeAck(ctx, s.db, activityID)
}

func latestScopeAck(ctx context.Context, q queryer, activityID int64) (*domain.ScopeAck, error) {
	var a domain.ScopeAck
	var date string
	err := q.QueryRowContext(ctx,
		`SELECT id, activity_id, date, baseline_count, last_subtask_id FROM scope_acks
		 WHERE activity_id = ? ORDER BY id DESC LIMIT 1`, activityID).
		Scan(&a.ID, &a.ActivityID, &date, &a.BaselineCount, &a.LastSubtaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest scope ack of activity %d: %w", activityID, err)
	}
	if a.Date, err = parseDate(date); err != nil {
		return nil, err
	}
	return &a, nil
}

// AckScope acknowledges the activity's scope change (SPEC §5.4) in one
// transaction: it inserts the ScopeAck and the scope_ack journal entry. It
// returns ErrNotFlagged, writing nothing, when the activity is not flagged.
func (s *Store) AckScope(ctx context.Context, activityID int64, today domain.Date) (domain.ScopeAck, error) {
	var ack domain.ScopeAck
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		a, err := getActivity(ctx, tx, activityID)
		if err != nil {
			return err
		}
		subs, err := querySubtasks(ctx, tx,
			`SELECT `+subtaskColumns+` FROM subtasks WHERE activity_id = ? ORDER BY id`, activityID)
		if err != nil {
			return err
		}
		prev, err := latestScopeAck(ctx, tx, activityID)
		if err != nil {
			return err
		}
		creep := domain.ScopeCreep(a, subs, prev, today)
		if !creep.Flagged {
			return fmt.Errorf("activity %d: %w", activityID, ErrNotFlagged)
		}
		ack = domain.ScopeAck{ActivityID: activityID, Date: today, BaselineCount: len(subs)}
		for _, st := range subs {
			if st.ID > ack.LastSubtaskID {
				ack.LastSubtaskID = st.ID
			}
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO scope_acks (activity_id, date, baseline_count, last_subtask_id) VALUES (?, ?, ?, ?)`,
			activityID, today.String(), ack.BaselineCount, ack.LastSubtaskID)
		if err != nil {
			return mapErr(err)
		}
		if ack.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		_, err = insertJournal(ctx, tx, domain.JournalEntry{
			ActivityID: activityID, Date: today, Kind: domain.KindScopeAck, Text: domain.AckJournalText(creep),
		})
		return err
	})
	if err != nil {
		return domain.ScopeAck{}, fmt.Errorf("acknowledge scope: %w", err)
	}
	return ack, nil
}
