package hookcfg

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Read returns the settings file content; a missing file yields (nil, false, nil).
func Read(path string) (data []byte, exists bool, err error) {
	data, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// Apply writes content to path atomically (temp file in the same directory + rename). If the
// file exists it is first copied to <path>.bak-<timestamp> (mode 0600) and its permissions are
// kept; a symlinked settings file is written through. It returns the backup path ("" when the
// file did not exist). A missing directory is created with mode 0700.
func Apply(path, content string, now time.Time) (backup string, err error) {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	mode := fs.FileMode(0o600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
		old, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		backup, err = writeBackup(path, old, now)
		if err != nil {
			return "", fmt.Errorf("backup failed, settings untouched: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return backup, err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return backup, err
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		cleanup()
		return backup, err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		cleanup()
		return backup, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return backup, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return backup, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return backup, err
	}
	return backup, nil
}

func writeBackup(path string, data []byte, now time.Time) (string, error) {
	base := path + ".bak-" + now.Format("20060102-150405")
	for i := 0; i < 100; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.Write(data)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			_ = os.Remove(name)
			return "", werr
		}
		return name, nil
	}
	return "", errors.New("could not find a free backup name")
}
