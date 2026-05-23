CREATE TABLE IF NOT EXISTS document_approvals (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL,
    document_id VARCHAR(128) NOT NULL,
    approved BOOLEAN NOT NULL,
    tx_hash VARCHAR(64) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX (user_id),
    INDEX (document_id)
);
