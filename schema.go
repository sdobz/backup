package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	db *sql.DB
}

func NewDB(filename string) (db *DB, err error) {
	sqlDb, err := sql.Open("sqlite3", filename)
	if err != nil {
		return nil, err
	}
	return &DB{sqlDb}, nil
}

func (db *DB) getVerification(id FileId) DataFileVerification {
	return DataFileVerification{}
}

func (db *DB) hasDedupeHash(hash FileDedupeHash) bool {
	return false
}

func (db *DB) hasVerificationHash(FileVerificationHash) bool {
	return false
}
