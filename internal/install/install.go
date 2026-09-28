// Package install finds or downloads the official simplex-chat binary.
package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

func Asset() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "simplex-chat-ubuntu-24_04-x86_64", nil
	case "linux/arm64":
		return "simplex-chat-ubuntu-24_04-aarch64", nil
	case "darwin/arm64":
		return "simplex-chat-macos-aarch64", nil
	case "darwin/amd64":
		return "simplex-chat-macos-x86-64", nil
	case "windows/amd64":
		return "simplex-chat-windows-x86-64", nil
	default:
		return "", fmt.Errorf("no official simplex-chat build for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// Bin returns the simplex-chat binary to launch.
// SIMPLEX_CHAT_BIN wins, then a copy previously installed by this tool, then PATH.
func Bin() (string, error) {
	if p := os.Getenv("SIMPLEX_CHAT_BIN"); p != "" {
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			return "", fmt.Errorf("SIMPLEX_CHAT_BIN %s is not a file", p)
		}
		return p, nil
	}
	installed := InstalledPath()
	if st, err := os.Stat(installed); err == nil && !st.IsDir() {
		return installed, nil
	}
	if p, err := exec.LookPath("simplex-chat"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("simplex-chat is not installed; run: simplex install")
}

func InstalledPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", "simplex-chat")
	}
	name := "simplex-chat"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(home, ".local", "share", "simplex", "bin", name)
}

func Download(ctx context.Context, log io.Writer) (string, error) {
	asset, err := Asset()
	if err != nil {
		return "", err
	}
	url := "https://github.com/simplex-chat/simplex-chat/releases/latest/download/" + asset
	dest := InstalledPath()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	fmt.Fprintf(log, "downloading %s\n", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "simplex-agent-cli")
	client := &http.Client{Timeout: 10 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", res.Status)
	}
	tmp := dest + ".partial"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), res.Body)
	closeErr := f.Close()
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	if closeErr != nil {
		os.Remove(tmp)
		return "", closeErr
	}
	if n < 1_000_000 {
		os.Remove(tmp)
		return "", fmt.Errorf("download is only %d bytes; the release asset name may have changed", n)
	}
	elf, err := os.Open(tmp)
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	magic := make([]byte, 4)
	_, _ = elf.Read(magic)
	elf.Close()
	if !executableMagic(magic) {
		os.Remove(tmp)
		return "", fmt.Errorf("download is not a %s executable", runtime.GOOS)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return "", err
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	fmt.Fprintf(log, "installed %s (%d bytes)\nsha256 %s\ncompare that hash with the GitHub release notes before trusting the binary\n", dest, n, sum)
	return dest, nil
}

func executableMagic(magic []byte) bool {
	if len(magic) < 4 {
		return false
	}
	switch runtime.GOOS {
	case "linux":
		return string(magic) == "\x7fELF"
	case "windows":
		return magic[0] == 'M' && magic[1] == 'Z'
	case "darwin":
		// Mach-O 64-bit, either endian, or a fat binary.
		return (magic[0] == 0xcf && magic[1] == 0xfa && magic[2] == 0xed && magic[3] == 0xfe) ||
			(magic[0] == 0xfe && magic[1] == 0xed && magic[2] == 0xfa && magic[3] == 0xcf) ||
			(magic[0] == 0xca && magic[1] == 0xfe && magic[2] == 0xba && magic[3] == 0xbe) ||
			(magic[0] == 0xbe && magic[1] == 0xba && magic[2] == 0xfe && magic[3] == 0xca)
	default:
		return false
	}
}
