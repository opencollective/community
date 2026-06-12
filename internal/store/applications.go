package store

import (
	"database/sql"
	"errors"
	"time"
)

// Application is one membership request (docs/flows/join.md).
type Application struct {
	ID         int64
	IdentityID int64
	Motivation string
	Newsletter bool
	Status     string // awaiting_email | pending | approved | declined
	Reason     string
	CreatedAt  int64
	DecidedAt  int64

	// joined from identities for rendering
	Username string
	Name     string
	Email    string
}

// CreateApplication starts an application in awaiting_email state.
func (c *Community) CreateApplication(identityID int64, motivation string, newsletter bool, now time.Time) (int64, error) {
	res, err := c.DB.Exec(
		`INSERT INTO applications (identity_id, motivation, newsletter, created_at) VALUES (?, ?, ?, ?)`,
		identityID, motivation, boolInt(newsletter), now.Unix(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// OpenApplicationByEmail finds an awaiting_email or pending application for
// an email (JOIN-09: one open application per email).
func (c *Community) OpenApplicationByEmail(email string) (*Application, error) {
	return c.applicationWhere(
		`i.email = ? AND a.status IN ('awaiting_email','pending')`, email)
}

func (c *Community) ApplicationByID(id int64) (*Application, error) {
	return c.applicationWhere(`a.id = ?`, id)
}

func (c *Community) applicationWhere(where string, arg any) (*Application, error) {
	row := c.DB.QueryRow(`
		SELECT a.id, a.identity_id, a.motivation, a.newsletter, a.status, a.reason,
		       a.created_at, COALESCE(a.decided_at, 0), i.username, i.name, COALESCE(i.email, '')
		FROM applications a JOIN identities i ON i.id = a.identity_id
		WHERE `+where+` ORDER BY a.created_at DESC LIMIT 1`, arg)
	return scanApplication(row)
}

func scanApplication(row *sql.Row) (*Application, error) {
	var a Application
	var nl int
	err := row.Scan(&a.ID, &a.IdentityID, &a.Motivation, &nl, &a.Status, &a.Reason,
		&a.CreatedAt, &a.DecidedAt, &a.Username, &a.Name, &a.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Newsletter = nl == 1
	return &a, nil
}

// PendingApplications lists applications awaiting review, oldest first.
func (c *Community) PendingApplications() ([]*Application, error) {
	rows, err := c.DB.Query(`
		SELECT a.id, a.identity_id, a.motivation, a.newsletter, a.status, a.reason,
		       a.created_at, COALESCE(a.decided_at, 0), i.username, i.name, COALESCE(i.email, '')
		FROM applications a JOIN identities i ON i.id = a.identity_id
		WHERE a.status = 'pending' ORDER BY a.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Application
	for rows.Next() {
		var a Application
		var nl int
		if err := rows.Scan(&a.ID, &a.IdentityID, &a.Motivation, &nl, &a.Status, &a.Reason,
			&a.CreatedAt, &a.DecidedAt, &a.Username, &a.Name, &a.Email); err != nil {
			return nil, err
		}
		a.Newsletter = nl == 1
		out = append(out, &a)
	}
	return out, rows.Err()
}

// SetApplicationStatus moves an application through its lifecycle.
func (c *Community) SetApplicationStatus(id int64, status, reason string, now time.Time) error {
	_, err := c.DB.Exec(
		`UPDATE applications SET status = ?, reason = ?, decided_at = ? WHERE id = ?`,
		status, reason, now.Unix(), id)
	return err
}

// MarkApplicationPending is called once the applicant verified their email.
func (c *Community) MarkApplicationPending(id int64) error {
	_, err := c.DB.Exec(
		`UPDATE applications SET status = 'pending' WHERE id = ? AND status = 'awaiting_email'`, id)
	return err
}

// RecordDecision stores one reviewer's signed-off decision. A reviewer
// decides an application at most once (JOIN-04).
func (c *Community) RecordDecision(applicationID, approverID int64, decision string, now time.Time) (bool, error) {
	res, err := c.DB.Exec(
		`INSERT OR IGNORE INTO application_approvals (application_id, approver_id, decision, created_at)
		 VALUES (?, ?, ?, ?)`, applicationID, approverID, decision, now.Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Deciders returns the approver ids who recorded the given decision.
func (c *Community) Deciders(applicationID int64, decision string) ([]int64, error) {
	rows, err := c.DB.Query(
		`SELECT approver_id FROM application_approvals WHERE application_id = ? AND decision = ?`,
		applicationID, decision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// LastDeclineFor returns when an identity's latest application was
// declined, or zero (JOIN-08: 30-day reapply window).
func (c *Community) LastDeclineFor(identityID int64) (time.Time, error) {
	var ts sql.NullInt64
	err := c.DB.QueryRow(
		`SELECT MAX(decided_at) FROM applications WHERE identity_id = ? AND status = 'declined'`,
		identityID).Scan(&ts)
	if err != nil || !ts.Valid {
		return time.Time{}, err
	}
	return time.Unix(ts.Int64, 0), nil
}
