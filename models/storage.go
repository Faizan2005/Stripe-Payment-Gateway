package models

import (
	"database/sql"
	"fmt"
	"time"
)

type Users struct {
	ID        uint      `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Email     string    `json:"email" db:"email"`
	StripeID  string    `json:"stripe_id" db:"stripe_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type Payment struct {
	ID              uint      `json:"id" db:"id"`
	UserID          uint      `json:"user_id" db:"user_id"`
	Name            string    `json:"name" db:"name"`
	Email           string    `json:"email" db:"email"`
	SubscriptionID  uint      `json:"subscription_id" db:"subscription_id"`
	TransactionID   uint      `json:"transaction_id" db:"transaction_id"`
	StripePaymentID string    `json:"stripe_payment_intent_id" db:"stripe_payment_intent_id"`
	Amount          int64     `json:"amount" db:"amount"`
	Currency        string    `json:"currency" db:"currency"`
	PaymentMethod   string    `json:"payment_method" db:"payment_method"`
	Status          string    `json:"status" db:"status"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type Refund struct {
	ID             uint      `json:"id" db:"id"`
	PaymentID      uint      `json:"payment_id" db:"payment_id"`
	TransactionID  uint      `json:"transaction_id" db:"transaction_id"`
	StripeRefundID string    `json:"stripe_refund_id" db:"stripe_refund_id"`
	Amount         int64     `json:"amount_refunded" db:"amount_refunded"`
	Status         string    `json:"status" db:"status"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

type Subscription struct {
	ID                   uint       `json:"id" db:"id"`
	UserID               uint       `json:"user_id" db:"user_id"`
	PaymentID            uint       `json:"payment_id" db:"payment_id"`
	Amount               int64      `json:"amount" db:"amount"`
	Currency             string     `json:"currency" db:"currency"`
	StripeSubscriptionID string     `json:"stripe_subscription_id" db:"stripe_subscription_id"`
	Status               string     `json:"status" db:"status"`
	StartDate            time.Time  `json:"start_date" db:"start_date"`
	EndDate              *time.Time `json:"end_date,omitempty" db:"end_date"`
}

type Transaction struct {
	ID              uint      `json:"id" db:"id"`
	UserID          uint      `json:"user_id" db:"user_id"`
	PaymentID       uint      `json:"payment_id,omitempty" db:"payment_id"`
	RefundID        uint      `json:"refund_id,omitempty" db:"refund_id"`
	Amount          int64     `json:"amount" db:"amount"`
	Currency        string    `json:"currency" db:"currency"`
	TransactionType string    `json:"transaction_type" db:"transaction_type"`
	Status          string    `json:"status" db:"status"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type Storage interface {
	CreatePayment(uint, string, string, int64, string, string, string) (uint, error)
	GetPaymentDetails(paymentintentID string) (*Payment, error)
	UpdatePaymentStatus(string, string) error
	CreateRefund(uint, int64, string, string) (uint, error)
	UpdateRefundStatus(string, string) error
	CancelPayment(uint, uint) error
	CreateSubscription(uint, uint, int64, string, string, string) error
	UpdateSubscriptionStatus(string, string) error
	GetSubscriptionDetails(string) (*Subscription, error)
	CancelSubscription(uint, uint) error
	LogTransaction(uint, string, int64, string, *uint) error
	GetUserTransactions(uint) ([]*Transaction, error)
	CreateCustomer(name, email, stripeID string) (string, uint, error)
	CheckCustomer(name, email string) (string, uint, error)
}

type PostgresStorage struct {
	db *sql.DB
}

func NewPostgresStorage(db *sql.DB) *PostgresStorage {
	return &PostgresStorage{
		db: db,
	}
}

func (s *PostgresStorage) CreatePayment(userID uint, name, email string, amount int64, currency string, method string, stripeID string) (uint, error) {
	query := `INSERT INTO payments (user_id, name, email, amount, currency, payment_method, stripe_payment_intent_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`

	var p Payment
	err := s.db.QueryRow(query, userID, name, email, amount, currency, method, stripeID).Scan(&p.ID)
	if err != nil {
		return 0, err
	}
	return p.ID, nil
}

func (s *PostgresStorage) CreateRefund(paymentID uint, amount int64, status string, stripeID string) (uint, error) {
	var refID uint
	query := `INSERT INTO refunds (payment_id, amount, status, stripe_refund_id)
VALUES ($1, $2, $3, $4)
RETURNING id`

	err := s.db.QueryRow(query, paymentID, amount, status, stripeID).Scan(&refID)
	if err != nil {
		return 0, err
	}

	return refID, nil
}

func (s *PostgresStorage) CreateSubscription(userID uint, paymentID uint, amount int64, currency string, stripeID string, status string) error {
	query := `INSERT INTO subscriptions (user_id, payment_id, amount, currency, stripe_subscription_id, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, payment_id, amount, currency, stripe_subscription_id, status`

	var sb Subscription
	err := s.db.QueryRow(query, userID, paymentID, amount, currency, stripeID, status).Scan(&sb.ID, &sb.UserID, &sb.PaymentID, &sb.Amount, &sb.Currency, &sb.StripeSubscriptionID, &sb.Status)

	return err
}

func (s *PostgresStorage) UpdatePaymentStatus(stripe_payment_intent_id string, status string) error {
	query := `UPDATE payments SET status=$1 WHERE stripe_payment_intent_id=$2`

	_, err := s.db.Exec(query, status, stripe_payment_intent_id)
	return err
}

func (s *PostgresStorage) CancelPayment(paymentID, userID uint) error {
	query := `UPDATE payments SET status='canceled' WHERE id=$1 AND user_id=$2`

	_, err := s.db.Exec(query, paymentID, userID)

	return err
}

func (s *PostgresStorage) CancelSubscription(subID, userID uint) error {
	query := `UPDATE subscriptions SET status='canceled' WHERE id=$1 AND user_id=$2`

	_, err := s.db.Exec(query, subID, userID)

	return err
}

func (s *PostgresStorage) UpdateSubscriptionStatus(stripe_subscription_id string, status string) error {
	query := `UPDATE subscriptions SET status=$1 WHERE stripe_subscription_id=$2`

	_, err := s.db.Exec(query, status, stripe_subscription_id)
	return err
}

func (s *PostgresStorage) LogTransaction(userID uint, txnType string, amount int64, currency string, refID *uint) error {
	query1 := `INSERT INTO transactions (user_id, transaction_type, amount, currency, payment_id) VALUES ($1, $2, $3, $4, $5) RETURNING id, user_id, transaction_type, amount, currency, payment_id`

	query2 := `INSERT INTO transactions (user_id, transaction_type, amount, currency, refund_id) VALUES ($1, $2, $3, $4, $5) RETURNING id, user_id, transaction_type, amount, currency, refund_id`

	var t Transaction

	switch txnType {
	case "payment":
		err := s.db.QueryRow(query1, userID, txnType, amount, currency, refID).Scan(&t.ID, &t.UserID, &t.TransactionType, &t.Amount, &t.Currency, &t.PaymentID)

		return err

	case "refund":
		err := s.db.QueryRow(query2, userID, txnType, amount, currency, refID).Scan(&t.ID, &t.UserID, &t.TransactionType, &t.Amount, &t.Currency, &t.RefundID)

		return err
	}

	return nil
}

func (s *PostgresStorage) GetUserTransactions(userID uint) ([]*Transaction, error) {
	query := "SELECT * FROM transactions WHERE user_id=$1"

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var ts []*Transaction

	for rows.Next() {
		var t Transaction
		err := rows.Scan(&t.ID, &t.UserID, &t.PaymentID, &t.RefundID, &t.Amount, &t.Currency, &t.TransactionType, &t.Status, &t.CreatedAt)
		if err != nil {
			return nil, err
		}
		ts = append(ts, &t)
	}

	return ts, nil
}

func (s *PostgresStorage) CreateCustomer(name, email, stripeID string) (string, uint, error) {
	var userID uint

	// User does not exist, so insert a new one
	query := `INSERT INTO users (name, email, stripe_id) VALUES ($1, $2, $3) RETURNING id`
	err := s.db.QueryRow(query, name, email, stripeID).Scan(&userID)
	if err != nil {
		return "", 0, err
	}

	return stripeID, userID, nil
}

func (s *PostgresStorage) CheckCustomer(name, email string) (string, uint, error) {
	var userID uint
	var stripeID string

	// Check if user already exists
	query := `SELECT stripe_id, id FROM users WHERE name=$1 AND email=$2`
	err := s.db.QueryRow(query, name, email).Scan(&userID, &stripeID)
	if err == nil {
		return stripeID, userID, nil // User already exists, return ID
	}

	return "", 0, err
}

func (s *PostgresStorage) GetPaymentDetails(paymentintentID string) (*Payment, error) {
	query := `SELECT * FROM payments WHERE stripe_payment_intent_id=$1`

	var p Payment
	err := s.db.QueryRow(query, paymentintentID).Scan(&p.ID, &p.UserID, &p.Name, &p.Email, &p.SubscriptionID, &p.TransactionID, &p.StripePaymentID, &p.Amount, &p.Currency, &p.PaymentMethod, &p.Status, &p.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no payment found for payment intent ID: %s", paymentintentID)
		}
		return nil, err
	}

	return &p, nil
}

func (s *PostgresStorage) GetSubscriptionDetails(subID string) (*Subscription, error) {
	query := `SELECT * FROM subscriptions WHERE stripe_subscription_id=$1`

	var sub Subscription
	err := s.db.QueryRow(query, subID).Scan(&sub)
	if err != nil {
		return nil, err
	}

	return &sub, nil
}

func (s *PostgresStorage) UpdateRefundStatus(stripeRefundID, status string) error {
	query := `UPDATE refunds SET status=$1 WHERE stripe_refund_id=$2`

	_, err := s.db.Exec(query, status, stripeRefundID)
	return err
}
