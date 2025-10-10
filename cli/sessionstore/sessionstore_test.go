package sessionstore_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"runtime"
	"testing"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cli/sessionstore"
)

type mockKeyringProvider struct {
	err error
}

func (u mockKeyringProvider) Set(_, _, _ string) error { return u.err }

func (u mockKeyringProvider) Get(_, _ string) (string, error) { return "", u.err }

func (u mockKeyringProvider) Delete(_, _ string) error { return u.err }

func TestOSKeyringWithFileFallback_OS_Keyring(t *testing.T) {
	t.Parallel()

	// This test exercises use of the operating system keyring. As a result,
	// the operating system keyring is expected to be available.
	dir := t.TempDir()
	cfg := config.Root(dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	srvURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	expSessionFile := path.Join(dir, "session")

	backend := sessionstore.NewKeyringWithFileFallback()
	if loc := backend.PreferredLocation(); loc != sessionstore.LocationKeyring {
		t.Fatalf("got preferred location %v, want %v", loc, sessionstore.LocationKeyring)
	}
	_, _, err = backend.Read(cfg, srvURL)
	if !os.IsNotExist(err) {
		t.Fatal("expected error when reading and no token in keyring or on disk")
	}
	err = backend.Delete(cfg, srvURL)
	if !xerrors.Is(err, os.ErrNotExist) {
		t.Fatal("expected error when deleting and no token in keyring or on disk")
	}

	// Write a token and read it back.
	const inputToken = "12345abc"
	src, err := backend.Write(cfg, srvURL, inputToken)
	if err != nil {
		t.Fatal(err)
	}
	wantSrc := sessionstore.LocationKeyring
	if src != wantSrc {
		t.Fatalf("got source %v want %v", src, wantSrc)
	}
	if _, err = os.Stat(expSessionFile); !xerrors.Is(err, os.ErrNotExist) {
		t.Fatal("expected session token file to not exist")
	}

	token, src, err := backend.Read(cfg, srvURL)
	if err != nil {
		t.Fatalf("unexpected error reading token: %v", err)
	}
	if src != wantSrc {
		t.Fatalf("got source %v want %v", src, wantSrc)
	}
	if token != inputToken {
		t.Fatalf("got token %v want %v", token, inputToken)
	}

	// Delete the token and attempt to read it back.
	err = backend.Delete(cfg, srvURL)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = backend.Read(cfg, srvURL)
	if !os.IsNotExist(err) {
		t.Fatal("expected error when no token in keyring or on disk")
	}
}

func TestOSKeyringWithFileFallback_Fallback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfg := config.Root(dir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	srvURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	expSessionFile := path.Join(dir, "session")

	errKeyring := xerrors.New("not available")
	backend := sessionstore.NewKeyringProviderWithFileFallback(mockKeyringProvider{err: errKeyring})
	_, _, err = backend.Read(cfg, srvURL)
	if !os.IsNotExist(err) {
		t.Fatal("expected error when reading and no token in keyring or on disk")
	}
	err = backend.Delete(cfg, srvURL)
	if runtime.GOOS == "linux" {
		if err != nil {
			// Linux is special in this case and returns no error. See Delete for an
			// explanation.
			t.Fatal("Linux expected error when deleting and no token in keyring or on disk")
		}
	} else if !xerrors.Is(err, os.ErrNotExist) {
		t.Fatal("expected error when deleting and no token in keyring or on disk")
	}

	// Write a token and read it back.
	const inputToken = "abc1234"
	src, err := backend.Write(cfg, srvURL, inputToken)
	if err != nil {
		t.Fatal(err)
	}
	wantSrc := sessionstore.LocationFile
	if src != wantSrc {
		t.Fatalf("got source %v want %v", src, wantSrc)
	}
	if _, err = os.Stat(expSessionFile); err != nil {
		t.Fatal(err)
	}

	token, src, err := backend.Read(cfg, srvURL)
	if err != nil {
		t.Fatalf("unexpected error reading token: %v", err)
	}
	if src != wantSrc {
		t.Fatalf("got source %v want %v", src, wantSrc)
	}
	if token != inputToken {
		t.Fatalf("got token %v want %v", token, inputToken)
	}

	// Delete the token and attempt to read it back.
	err = backend.Delete(cfg, srvURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(expSessionFile); !xerrors.Is(err, os.ErrNotExist) {
		t.Fatal("expected file to be removed")
	}
	_, _, err = backend.Read(cfg, srvURL)
	if !os.IsNotExist(err) {
		t.Fatal("expected error when no token in keyring or on disk")
	}
}
