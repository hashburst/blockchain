package database

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
)

type MySQLConn struct {
	DB *sql.DB
}

func NewMySQLConn(dsn string) (*MySQLConn, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("connection error: %v", err)
	}

	return &MySQLConn{DB: db}, nil
}

func (m *MySQLConn) LogDocumentApproval(txHash string, userId string, docId string, approved bool) error {
	_, err := m.DB.Exec(
		"INSERT INTO document_approvals (user_id, document_id, approved, tx_hash) VALUES (?, ?, ?, ?)",
		userId, docId, approved, txHash,
	)
	return err
}
