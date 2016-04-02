package main

import (
	"database/sql"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"log"
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

// Row structs
type pragmaInfo struct {
	cid        int
	name       []byte
	_type      []byte
	notnull    bool
	dflt_value []byte
	pk         int
}

type FileMeta struct {
	Identity   string
	BackupName string
	Session    string
	FileName   string
}

func (m *FileMeta) Path() (string, error) {
	if m.Identity == "" {
		return "", errors.New("Identity is blank")
	}
	if m.BackupName == "" {
		return "", errors.New("BackupName is blank")
	}
	if m.Session == "" {
		return "", errors.New("Session is blank")
	}
	if m.FileName == "" {
		return "", errors.New("FileName is blank")
	}

	return path.Join(m.Identity, m.BackupName, m.Session, m.FileName), nil
}

func (m *FileMeta) DataExists() (bool, error) {
	path, err := m.Path()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	return err == nil, err
}

func (m *FileMeta) Writer() (io.WriteCloser, error) {
	// TODO: Permissions, modified
	filePath, err := m.Path()
	if err != nil {
		return nil, err
	}
	os.MkdirAll(path.Dir(filePath), 0700)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return file, nil
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
	// > .schema files
	// CREATE TABLE files (path string, filename string, dedupe varchar(32), verif varchar(32), modified datetime);
	defer rows.Close()
	expectedColumns := []string{
		"path",     // Path to file relative to db
		"filename", // Original filename relative to backup config
		"dedupe",   // imohash of file
		"verif",    // full hash of file
		"modified", // modification time of file
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
			return errors.New("Columns did not match expected")
		}
		i++
	}
	return nil
}

func (s *Storage) LinkDedupe(dedupe FileDedupeHash, linkMeta *FileMeta) (bool, error) {
	sourceMeta, err := s.lookupFileMetaFromDedupe(dedupe)
	if err != nil {
		return false, err
	}
	if sourceMeta == nil {
		return false, nil
	}
	sourceFilename, err := sourceMeta.Path()
	if err != nil {
		return false, err
	}

	newFilename, err := linkMeta.Path()
	if err != nil {
		return false, err
	}

	if newFilename == sourceFilename {
		return true, nil
	}

	if err := os.MkdirAll(path.Dir(newFilename), 0700); err != nil {
		return false, err
	}

	err = os.Link(sourceFilename, newFilename)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Storage) LinkFromOtherSession(dest *FileMeta) (bool, error) {
	sourceMeta, err := s.lookupFileMetaFromOtherSession(dest)
	if err != nil {
		return false, err
	}
	if sourceMeta == nil {
		return false, nil
	}
	sourceFilename, err := sourceMeta.Path()
	if err != nil {
		return false, err
	}

	newFilename, err := dest.Path()
	if err != nil {
		return false, err
	}

	if newFilename == sourceFilename {
		return true, nil
	}

	if err := os.MkdirAll(path.Dir(newFilename), 0700); err != nil {
		return false, err
	}

	err = os.Link(sourceFilename, newFilename)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Storage) WriteChunk(dest *FileMeta, chunk []byte) error {
	w, err := dest.Writer()
	if w != nil {
		defer w.Close()
	}
	if err != nil {
		return err
	}

	if _, err = w.Write(chunk); err != nil {
		return err
	}
	return nil
}

func (s *Storage) lookupFileMetaFromDedupe(dedupe FileDedupeHash) (*FileMeta, error) {
	meta := &FileMeta{}
	rows, err := s.db.Query("SELECT identity, backup_name, session, file_name FROM meta WHERE dedupe = '?'", dedupe)
	defer rows.Close()
	if err != nil {
		log.Fatal("Query failed on lookupFileMetaFromDedupe")
	}

	for rows.Next() {
		err := rows.Scan(&meta.Identity, &meta.BackupName, &meta.Session, &meta.FileName)
		if err != nil {
			return nil, err
			break
		}
		return meta, nil
	}
	return nil, errors.New("No results")
}

func (s *Storage) lookupFileMetaFromOtherSession(search *FileMeta) (*FileMeta, error) {
	meta := &FileMeta{}
	rows, err := s.db.Query("SELECT identity, backup_name, session, file_name FROM meta WHERE identity = '?' AND backup_name = '?' AND file_name = '?'", search.Identity, search.BackupName, search.FileName)
	defer rows.Close()
	if err != nil {
		log.Fatal("Query failed on lookupFileMetaFromOtherSession")
	}

	for rows.Next() {
		err := rows.Scan(&meta.Identity, &meta.BackupName, &meta.Session, &meta.FileName)
		if err != nil {
			return nil, err
			break
		}
		return meta, nil
	}
	return nil, errors.New("No results")

}
