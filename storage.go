package main

import (
	"database/sql"
	"encoding/hex"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path"
)

type Storage struct {
	db   *sql.DB
	base string
}

func NewStorage(base string) (storage *Storage, err error) {
	dbPath := path.Join(base, "meta.sqlite3")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, err
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := verifyDB(db); err != nil {
		return nil, err
	}
	storage = &Storage{
		db:   db,
		base: base,
	}

	return storage, nil
}

type pragmaInfo struct {
	cid        int
	name       []byte
	_type      []byte
	notnull    bool
	dflt_value []byte
	pk         int
}

func verifyDB(db *sql.DB) error {
	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table'")
	defer rows.Close()
	if err != nil {
		return err
	}

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
	defer rows.Close()
	expectedColumns := []string{
		"filename",
		"dedupe",
		"verif",
		"modified",
	}
	i := 0
	var info pragmaInfo
	for rows.Next() {
		err := rows.Scan(&info.cid,
			&info.name,
			&info._type,
			&info.notnull,
			&info.dflt_value,
			&info.pk)
		if err != nil {
			return err
		}
		if expectedColumns[i] != string(info.name) {
			return errors.New("Schema did not match expected")
		}
		i++
	}
	return nil
}

func (s *Storage) StoreBinary(fingerprint Fingerprint, name string, filename string, chunk []byte) {
	// TODO: maintain mode
	fileBase := path.Join(s.base, name, hex.EncodeToString(fingerprint[:4]))

	os.MkdirAll(path.Join(fileBase, path.Dir(filename)), 0700)
	f, err := os.OpenFile(path.Join(fileBase, filename), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	if _, err = f.Write(chunk); err != nil {
		panic(err)
	}
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
