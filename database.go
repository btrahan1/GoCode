package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

type ChatMessage struct {
	Sender      string `json:"sender"` // "user" or "assistant"
	Text        string `json:"text"`
	Reasoning   string `json:"reasoning"`
	ImageBase64 string `json:"imageBase64"`
}

type SavedConversation struct {
	SessionID   string        `json:"sessionId"`
	ActiveModel string        `json:"activeModel"`
	AgentMode   string        `json:"agentMode"`
	YoloMode    bool          `json:"yoloMode"`
	Messages    []ChatMessage `json:"messages"`
}

type DBClient struct {
	db         *sql.DB
	driverName string
}

func NewDBClient(connStr string) (*DBClient, error) {
	var db *sql.DB
	var err error
	var driverName string

	isPostgres := strings.HasPrefix(connStr, "postgres://")

	if isPostgres {
		driverName = "postgres"
		db, err = sql.Open("postgres", connStr)
		if err != nil {
			return nil, err
		}

		// Try to ping Postgres to verify connection with a few retries
		var pingErr error
		for i := 0; i < 5; i++ {
			pingErr = db.Ping()
			if pingErr == nil {
				break
			}
			log.Printf("Waiting for Postgres database... attempt %d/5: %v", i+1, pingErr)
			time.Sleep(2 * time.Second)
		}
		if pingErr != nil {
			return nil, fmt.Errorf("failed to connect to Postgres: %w", pingErr)
		}
	} else {
		// Use SQLite fallback
		driverName = "sqlite"
		
		// Find user configuration dir
		configDir, err := os.UserConfigDir()
		if err != nil {
			configDir = "." // Fallback to current working directory
		}

		appDir := filepath.Join(configDir, "GoCode")
		if err := os.MkdirAll(appDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create AppData configuration directory: %w", err)
		}

		dbPath := filepath.Join(appDir, "gocode.db")
		log.Printf("Connecting to SQLite database at: %s", dbPath)

		db, err = sql.Open("sqlite", dbPath)
		if err != nil {
			return nil, err
		}
	}

	client := &DBClient{db: db, driverName: driverName}
	if err := client.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return client, nil
}

func (c *DBClient) Close() error {
	return c.db.Close()
}

func (c *DBClient) initSchema() error {
	var queries []string

	if c.driverName == "sqlite" {
		queries = []string{
			`CREATE TABLE IF NOT EXISTS conversations (
				session_id VARCHAR(255) PRIMARY KEY,
				workspace_path TEXT NOT NULL,
				active_model VARCHAR(255) NOT NULL,
				agent_mode VARCHAR(255) NOT NULL,
				yolo_mode BOOLEAN NOT NULL DEFAULT FALSE,
				updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);`,
			`CREATE TABLE IF NOT EXISTS messages (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				session_id VARCHAR(255) REFERENCES conversations(session_id) ON DELETE CASCADE,
				sender VARCHAR(50) NOT NULL,
				text TEXT NOT NULL,
				reasoning TEXT NOT NULL DEFAULT '',
				image_base64 TEXT NOT NULL DEFAULT '',
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
			);`,
			`CREATE TABLE IF NOT EXISTS settings (
				key VARCHAR(255) PRIMARY KEY,
				value TEXT NOT NULL
			);`,
			`CREATE INDEX IF NOT EXISTS idx_conversations_workspace ON conversations(workspace_path);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);`,
		}
	} else {
		queries = []string{
			`CREATE TABLE IF NOT EXISTS conversations (
				session_id VARCHAR(255) PRIMARY KEY,
				workspace_path TEXT NOT NULL,
				active_model VARCHAR(255) NOT NULL,
				agent_mode VARCHAR(255) NOT NULL,
				yolo_mode BOOLEAN NOT NULL DEFAULT FALSE,
				updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			);`,
			`CREATE TABLE IF NOT EXISTS messages (
				id SERIAL PRIMARY KEY,
				session_id VARCHAR(255) REFERENCES conversations(session_id) ON DELETE CASCADE,
				sender VARCHAR(50) NOT NULL,
				text TEXT NOT NULL,
				reasoning TEXT NOT NULL DEFAULT '',
				image_base64 TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			);`,
			`CREATE TABLE IF NOT EXISTS settings (
				key VARCHAR(255) PRIMARY KEY,
				value TEXT NOT NULL
			);`,
			`CREATE INDEX IF NOT EXISTS idx_conversations_workspace ON conversations(workspace_path);`,
			`CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);`,
		}
	}

	for _, q := range queries {
		if _, err := c.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// LoadConversation retrieves a conversation and its messages for a given workspace path.
func (c *DBClient) LoadConversation(workspacePath string) (*SavedConversation, error) {
	var conv SavedConversation
	query := `SELECT session_id, active_model, agent_mode, yolo_mode 
	          FROM conversations WHERE workspace_path = $1 
	          ORDER BY updated_at DESC LIMIT 1`
	err := c.db.QueryRow(query, workspacePath).Scan(&conv.SessionID, &conv.ActiveModel, &conv.AgentMode, &conv.YoloMode)
	if err == sql.ErrNoRows {
		return nil, nil // No conversation found
	} else if err != nil {
		return nil, err
	}

	// Then load the messages
	msgQuery := `SELECT sender, text, reasoning, image_base64 
	             FROM messages WHERE session_id = $1 
	             ORDER BY id ASC`
	rows, err := c.db.Query(msgQuery, conv.SessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conv.Messages = []ChatMessage{}
	for rows.Next() {
		var msg ChatMessage
		if err := rows.Scan(&msg.Sender, &msg.Text, &msg.Reasoning, &msg.ImageBase64); err != nil {
			return nil, err
		}
		conv.Messages = append(conv.Messages, msg)
	}

	return &conv, nil
}

// SaveConversation saves (or updates) a conversation and all its messages.
func (c *DBClient) SaveConversation(workspacePath string, conv *SavedConversation) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Upsert conversation settings
	upsertConv := `
		INSERT INTO conversations (session_id, workspace_path, active_model, agent_mode, yolo_mode, updated_at)
		VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		ON CONFLICT (session_id) DO UPDATE 
		SET active_model = EXCLUDED.active_model,
		    agent_mode = EXCLUDED.agent_mode,
		    yolo_mode = EXCLUDED.yolo_mode,
		    updated_at = CURRENT_TIMESTAMP;`
	
	_, err = tx.Exec(upsertConv, conv.SessionID, workspacePath, conv.ActiveModel, conv.AgentMode, conv.YoloMode)
	if err != nil {
		return err
	}

	// Delete old messages for this session
	_, err = tx.Exec(`DELETE FROM messages WHERE session_id = $1`, conv.SessionID)
	if err != nil {
		return err
	}

	// Insert new messages
	if len(conv.Messages) > 0 {
		insertMsg := `INSERT INTO messages (session_id, sender, text, reasoning, image_base64, created_at) VALUES `
		vals := []interface{}{}
		for i, msg := range conv.Messages {
			n := i * 5
			insertMsg += fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, CURRENT_TIMESTAMP),", n+1, n+2, n+3, n+4, n+5)
			vals = append(vals, conv.SessionID, msg.Sender, msg.Text, msg.Reasoning, msg.ImageBase64)
		}
		insertMsg = insertMsg[:len(insertMsg)-1] // strip last comma
		_, err = tx.Exec(insertMsg, vals...)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteConversation deletes conversation and cascade deletes messages.
func (c *DBClient) DeleteConversation(workspacePath string) error {
	_, err := c.db.Exec(`DELETE FROM conversations WHERE workspace_path = $1`, workspacePath)
	return err
}

func (c *DBClient) GetSetting(key string) (string, error) {
	var val string
	err := c.db.QueryRow(`SELECT value FROM settings WHERE key = $1`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (c *DBClient) SaveSetting(key, value string) error {
	_, err := c.db.Exec(`
		INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`, key, value)
	return err
}

func (c *DBClient) ListConversations(workspacePath string) ([]SavedConversation, error) {
	rows, err := c.db.Query(`
		SELECT session_id, active_model, agent_mode, yolo_mode 
		FROM conversations 
		WHERE workspace_path = $1 
		ORDER BY updated_at DESC
	`, workspacePath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []SavedConversation
	for rows.Next() {
		var conv SavedConversation
		err := rows.Scan(&conv.SessionID, &conv.ActiveModel, &conv.AgentMode, &conv.YoloMode)
		if err != nil {
			return nil, err
		}
		conv.Messages = []ChatMessage{}
		list = append(list, conv)
	}
	return list, nil
}

func (c *DBClient) LoadSpecificConversation(sessionID string) (*SavedConversation, error) {
	var conv SavedConversation
	query := `SELECT session_id, active_model, agent_mode, yolo_mode 
	          FROM conversations WHERE session_id = $1`
	err := c.db.QueryRow(query, sessionID).Scan(&conv.SessionID, &conv.ActiveModel, &conv.AgentMode, &conv.YoloMode)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	msgQuery := `SELECT sender, text, reasoning, image_base64 
	             FROM messages WHERE session_id = $1 
	             ORDER BY id ASC`
	rows, err := c.db.Query(msgQuery, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conv.Messages = []ChatMessage{}
	for rows.Next() {
		var msg ChatMessage
		if err := rows.Scan(&msg.Sender, &msg.Text, &msg.Reasoning, &msg.ImageBase64); err != nil {
			return nil, err
		}
		conv.Messages = append(conv.Messages, msg)
	}

	return &conv, nil
}
