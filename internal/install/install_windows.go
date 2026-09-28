//go:build windows

package install

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	opensslZipURL = "https://download.firedaemon.com/FireDaemon-OpenSSL/openssl-3.0.22.zip"
	opensslZipSum = "323fa7e2062b81fe5f4becd02e885b138f5cd2a262eea7dd30331f12f54d0573"
)

// prepareRuntime puts the OpenSSL 3 DLLs the official Windows simplex-chat
// binary imports beside that exe, then refuses the install if it still cannot
// load. 0xc0000135 is STATUS_DLL_NOT_FOUND.
func prepareRuntime(ctx context.Context, dest string, log io.Writer) error {
	dir := filepath.Dir(dest)
	windows.SetErrorMode(windows.SEM_FAILCRITICALERRORS | windows.SEM_NOGPFAULTERRORBOX | windows.SEM_NOOPENFILEERRORBOX)
	if err := ensureDLL(ctx, dir, "libcrypto-3-x64.dll", log); err != nil {
		return err
	}
	code, err := probeChat(dest)
	if err != nil {
		return err
	}
	if dllNotFound(code) {
		if err := ensureDLL(ctx, dir, "libssl-3-x64.dll", log); err != nil {
			return err
		}
		code, err = probeChat(dest)
		if err != nil {
			return err
		}
	}
	if dllNotFound(code) {
		return fmt.Errorf("simplex-chat still exits 0xc0000135 after installing libcrypto-3-x64.dll and libssl-3-x64.dll next to %s", dest)
	}
	fmt.Fprintf(log, "simplex-chat starts\n")
	return nil
}

func ensureDLL(ctx context.Context, dir, name string, log io.Writer) error {
	dest := filepath.Join(dir, name)
	if st, err := os.Stat(dest); err == nil && !st.IsDir() && st.Size() > 0 {
		return nil
	}
	zipPath, err := opensslZip(ctx, log)
	if err != nil {
		return err
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	suffix := "x64/bin/" + name
	for _, f := range zr.File {
		clean := strings.ReplaceAll(f.Name, "\\", "/")
		if !strings.HasSuffix(clean, suffix) || f.FileInfo().IsDir() {
			continue
		}
		return extractDLL(f, dest)
	}
	return fmt.Errorf("openssl zip has no %s", name)
}

func opensslZip(ctx context.Context, log io.Writer) (string, error) {
	dest := filepath.Join(os.TempDir(), "simplex-openssl-3.0.22.zip")
	if st, err := os.Stat(dest); err == nil && !st.IsDir() {
		if sum, err := fileSum(dest); err == nil && sum == opensslZipSum {
			return dest, nil
		}
	}
	fmt.Fprintf(log, "downloading OpenSSL 3.0.22 for libcrypto-3-x64.dll\n")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opensslZipURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "simplex-agent-cli")
	res, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openssl download failed: %s", res.Status)
	}
	tmp := dest + ".partial"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, hash), res.Body)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return "", copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return "", closeErr
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if sum != opensslZipSum {
		os.Remove(tmp)
		return "", fmt.Errorf("openssl zip sha256 %s, want %s", sum, opensslZipSum)
	}
	os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return dest, nil
}

func extractDLL(f *zip.File, dest string) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	tmp := dest + ".partial"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, r)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	os.Remove(dest)
	return os.Rename(tmp, dest)
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// probeChat reports the process exit code. A still-running process is killed
// and reported as exit 0: the DLLs loaded and chat did not die at startup.
func probeChat(exe string) (int, error) {
	cmd := exec.Command(exe, "-h")
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return 0, nil
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return 0, err
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		return 0, nil
	}
}
