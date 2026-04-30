package storeutil

import (
	"os"
	"path/filepath"

	kleio "github.com/kleio-build/kleio-core"
	"github.com/kleio-build/kleio-cli/internal/apistore"
	"github.com/kleio-build/kleio-cli/internal/client"
	"github.com/kleio-build/kleio-cli/internal/config"
	"github.com/kleio-build/kleio-cli/internal/localdb"
)

// KleioDir is the project-local directory where Kleio stores its SQLite
// database and local config overrides.
const KleioDir = ".kleio"

// DBFile is the SQLite database filename inside KleioDir.
const DBFile = "kleio.db"

// Resolve returns the appropriate Store based on the current config state
// and working directory. The resolution order:
//
//  1. Auth configured (token or API key + workspace) → cloud mode (apistore)
//  2. .kleio/ directory exists in cwd or ancestors → local mode (localdb)
//  3. Neither → local mode with auto-init at cwd
func Resolve(cfg *config.Config, getClient func() *client.Client) (kleio.Store, error) {
	if config.HasAuth(cfg) && config.HasWorkspace(cfg) {
		return apistore.New(getClient()), nil
	}
	return resolveLocal()
}

// ResolveLocal forces local mode regardless of auth state. Returns a localdb.Store
// backed by the .kleio/kleio.db found in cwd or ancestors.
func ResolveLocal() (kleio.Store, error) {
	return resolveLocal()
}

func resolveLocal() (kleio.Store, error) {
	dbPath, err := FindDBPath()
	if err != nil {
		return nil, err
	}
	db, err := localdb.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return localdb.New(db), nil
}

// FindDBPath walks up from the current working directory looking for an
// existing .kleio/kleio.db. If none is found, returns a path rooted at cwd.
func FindDBPath() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := wd
	for {
		candidate := filepath.Join(dir, KleioDir, DBFile)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return filepath.Join(wd, KleioDir, DBFile), nil
}
