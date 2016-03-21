package main

import (
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"path"
)

type Storage struct {
	db *sql.DB
}

func NewStorage(base string) (storage *Storage, err error) {
	db, err := sql.Open("sqlite3", path.Join(base, "meta.sqlite3"))
	if err != nil {
		return nil, err
	}

	if err := verifyDB(db); err != nil {
		return nil, err
	}
	storage = &Storage{db}

	return storage, nil
}

func verifyDB(db *sql.DB) error {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return err
	}
	defer rows.Close()

	var tableName string
	for rows.Next() {
		err := rows.Scan(&tableName)
		if err != nil {
			return err
		}
		if tableName != "files" {
			return errors.New("Table that isn't files found")
		}
	}

	rows, err = db.Query("pragma table_info('files')")
	expectedColumns := []string{
		"filename",
		"dedupe",
		"verif",
		"modified",
	}
	i := 0
	var columnName string
	for rows.Next() {
		err := rows.Scan(&columnName)
		if err != nil {
			return err
		}
		if expectedColumns[i] != columnName {
			return errors.New("Schema did not match expected")
		}
		i++
	}
	return nil
}

func (s *Storage) GetVerification(id FileId) FileVerificationHash {
	return FileVerificationHash{}
}

func (s *Storage) HasDedupeHash(hash FileDedupeHash) bool {
	return false
}

func (s *Storage) HasVerificationHash(hash FileVerificationHash) bool {
	return false
}
