package cpa

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CpaAuthFile struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	AccountID    string `json:"account_id"`
	Email        string `json:"email"`
	Disabled     bool   `json:"disabled"`
	Expired      string `json:"expired"`
	LastRefresh  string `json:"last_refresh"`
	Type         string `json:"type"`
}

func AccountKeyFromFile(f *CpaAuthFile) string {
	plan := "unknown"
	if info, err := ParseAccountInfo(f.AccessToken); err == nil && info.PlanType != "" {
		plan = info.PlanType
	}
	return fmt.Sprintf("%s-%s-%s", f.Type, f.Email, plan)
}

func validateAccountKey(key string) error {
	if key == "" {
		return fmt.Errorf("empty account key")
	}
	if strings.Contains(key, "/") || strings.Contains(key, "\\") || strings.Contains(key, "..") {
		return fmt.Errorf("invalid account key: %q", key)
	}
	return nil
}

func ReadAuthFile(dir, accountKey string) (*CpaAuthFile, error) {
	if err := validateAccountKey(accountKey); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, accountKey+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f CpaAuthFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &f, nil
}

func WriteAuthFile(dir string, f *CpaAuthFile, accountKey string) error {
	if err := validateAccountKey(accountKey); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, accountKey+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ScanAuthDir(dir string) ([]CpaAuthFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []CpaAuthFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var f CpaAuthFile
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		files = append(files, f)
	}
	return files, nil
}

// AccountKeyFromFilename extracts the account key from a filename (without .json extension).
func AccountKeyFromFilename(filename string) string {
	return strings.TrimSuffix(filename, ".json")
}

// ScanAuthDirWithKeys returns parsed files along with their account keys derived from filenames.
type ScannedAccount struct {
	Key  string
	File CpaAuthFile
}

func ScanAuthDirKeyed(dir string) ([]ScannedAccount, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var result []ScannedAccount
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var f CpaAuthFile
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		key := AccountKeyFromFilename(e.Name())
		result = append(result, ScannedAccount{Key: key, File: f})
	}
	return result, nil
}

type DetailedScannedAccount struct {
	Key        string
	Path       string
	ModifiedAt time.Time
	File       CpaAuthFile
}

func ScanAuthDirDetailed(dir string) ([]DetailedScannedAccount, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var result []DetailedScannedAccount
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var f CpaAuthFile
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, DetailedScannedAccount{
			Key:        AccountKeyFromFilename(e.Name()),
			Path:       path,
			ModifiedAt: info.ModTime(),
			File:       f,
		})
	}
	return result, nil
}
