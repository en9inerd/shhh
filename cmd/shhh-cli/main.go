// Command shhh-cli pushes, pulls, and watches encrypted channel messages.
//
// Usage:
//
//	shhh-cli [--server URL] [--passphrase PHRASE] [--name DEVICE_NAME] <command> <uuid>
//
//	push <uuid> [text]       Encrypt text and push to channel (reads stdin if omitted)
//	push <uuid> --file PATH  Encrypt file and push to channel
//	pull <uuid>              Fetch and decrypt all queued messages; print text or save files
//	watch <uuid>             Connect SSE; receive messages and send by typing (interactive)
//
// Passphrase priority: --passphrase flag → SHHH_PASSPHRASE env → interactive prompt.
// Device name priority: --name flag → SHHH_DEVICE_NAME env → anonymous.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/en9inerd/shhh/internal/channel"
	"github.com/en9inerd/shhh/internal/util"
	"golang.org/x/term"
)

var version = "dev"

func versionString() string {
	var revision string
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, kv := range info.Settings {
			if kv.Key == "vcs.revision" && len(kv.Value) >= 7 {
				revision = kv.Value[:7]
			}
		}
	}
	s := "shhh-cli version " + version
	if revision != "" {
		s += " (" + revision + ")"
	}
	return s
}

const (
	msgTypeText = byte(0x01)
	msgTypeFile = byte(0x02)
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	for _, a := range args {
		if a == "--version" || a == "-version" {
			fmt.Println(versionString())
			return nil
		}
	}

	var serverURL, passphrase, deviceName, filePath string

	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		switch {
		case args[0] == "--server" && len(args) > 1:
			serverURL = args[1]
			args = args[2:]
		case args[0] == "--passphrase" && len(args) > 1:
			passphrase = args[1]
			args = args[2:]
		case args[0] == "--name" && len(args) > 1:
			deviceName = args[1]
			args = args[2:]
		case args[0] == "--file" && len(args) > 1:
			filePath = args[1]
			args = args[2:]
		default:
			return fmt.Errorf("unknown flag: %s", args[0])
		}
	}

	if len(args) < 2 {
		return fmt.Errorf("usage: shhh-cli [flags] <push|pull|watch> <uuid>")
	}
	cmd, uuid := args[0], args[1]
	textArg := ""
	if len(args) > 2 {
		textArg = strings.Join(args[2:], " ")
	}

	if !channel.IsValidUUID(uuid) {
		return fmt.Errorf("invalid UUID: must be 32-character lowercase hex")
	}

	if serverURL == "" {
		serverURL = os.Getenv("SHHH_SERVER")
	}
	if serverURL == "" {
		serverURL = "http://localhost:8000"
	}
	serverURL = strings.TrimRight(serverURL, "/")

	if passphrase == "" {
		passphrase = os.Getenv("SHHH_PASSPHRASE")
	}
	if passphrase == "" {
		var err error
		passphrase, err = readPassphrase("Passphrase: ")
		if err != nil {
			return fmt.Errorf("read passphrase: %w", err)
		}
	}
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}

	if deviceName == "" {
		deviceName = os.Getenv("SHHH_DEVICE_NAME")
	}
	if deviceName == "" {
		var b [2]byte
		if _, err := rand.Read(b[:]); err == nil {
			deviceName = fmt.Sprintf("anon-%x", b)
		} else {
			deviceName = "anon"
		}
	}
	// Truncate device name to 32 UTF-8 bytes.
	deviceName = util.TruncateUTF8(deviceName, 32)

	seen := make(map[string]bool)

	switch cmd {
	case "push":
		return cmdPush(ctx, serverURL, uuid, passphrase, deviceName, filePath, textArg)
	case "pull":
		return cmdPull(ctx, serverURL, uuid, passphrase, seen)
	case "watch":
		return cmdWatch(ctx, serverURL, uuid, passphrase, deviceName, seen)
	default:
		return fmt.Errorf("unknown command %q; use push, pull, or watch", cmd)
	}
}

// cmdPush encrypts and pushes text or a file to the channel.
func cmdPush(ctx context.Context, serverURL, uuid, passphrase, deviceName, filePath, textArg string) error {
	var payload []byte
	var msgType byte

	if filePath != "" {
		fileData, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		filename := util.SanitizeFilename(filepath.Base(filePath))
		fnBytes := []byte(filename) // SanitizeFilename already caps at 255 bytes
		payload = make([]byte, 2+len(fnBytes)+len(fileData))
		binary.BigEndian.PutUint16(payload[0:2], uint16(len(fnBytes)))
		copy(payload[2:], fnBytes)
		copy(payload[2+len(fnBytes):], fileData)
		msgType = msgTypeFile
	} else {
		if textArg == "" {
			fmt.Fprint(os.Stderr, "Message (end with Ctrl+D):\n")
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			textArg = string(data)
		}
		payload = []byte(textArg)
		msgType = msgTypeText
	}

	envelope, err := buildEnvelope(msgType, deviceName, payload)
	if err != nil {
		return fmt.Errorf("build envelope: %w", err)
	}

	blob, err := channel.EncryptBlob(envelope, passphrase, uuid)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		serverURL+"/api/channel/"+uuid, bytes.NewReader(blob))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		fmt.Fprintln(os.Stderr, "pushed")
		return nil
	}
	return fmt.Errorf("push failed: HTTP %d", resp.StatusCode)
}

// cmdPull fetches and decrypts all queued messages.
func cmdPull(ctx context.Context, serverURL, uuid, passphrase string, seen map[string]bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		serverURL+"/api/channel/"+uuid, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("channel not found")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pull failed: HTTP %d", resp.StatusCode)
	}

	var body struct {
		Messages []struct {
			Blob     string `json:"blob"`
			PushedAt string `json:"pushed_at"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	for _, m := range body.Messages {
		handleMessage(m.Blob, m.PushedAt, uuid, passphrase, seen)
	}
	if len(body.Messages) == 0 {
		fmt.Fprintln(os.Stderr, "no messages")
	}
	return nil
}

// cmdWatch subscribes to SSE and decrypts messages in real time.
// It also reads stdin so the user can send messages without leaving watch mode:
//
//	<text>          — push a text message
//	:file /path     — push a file
func cmdWatch(ctx context.Context, serverURL, uuid, passphrase, deviceName string, seen map[string]bool) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		serverURL+"/api/channel/"+uuid+"/watch", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("channel not found")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("watch failed: HTTP %d", resp.StatusCode)
	}

	fmt.Fprintln(os.Stderr, "watching — type a message and Enter to send, :file /path to send a file, Ctrl+C to stop")

	// SSE receiver goroutine — signals done via sseErr.
	sseErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		var event, data string
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if event == "message" && data != "" {
					var msg struct {
						Blob     string `json:"blob"`
						PushedAt string `json:"pushed_at"`
					}
					if json.Unmarshal([]byte(data), &msg) == nil {
						handleMessage(msg.Blob, msg.PushedAt, uuid, passphrase, seen)
					}
				}
				event, data = "", ""
				continue
			}
			if after, ok := strings.CutPrefix(line, "event: "); ok {
				event = after
			} else if after, ok := strings.CutPrefix(line, "data: "); ok {
				data = after
			}
			// Comments (": keepalive") are ignored.
		}
		sseErr <- scanner.Err()
	}()

	// Stdin sender goroutine — closes stdinLines on EOF, sends error on scanner failure.
	type stdinLine struct {
		text string
		err  error
	}
	stdinLines := make(chan stdinLine)
	go func() {
		defer close(stdinLines)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			stdinLines <- stdinLine{text: scanner.Text()}
		}
		if err := scanner.Err(); err != nil {
			stdinLines <- stdinLine{err: err}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-sseErr:
			return err
		case line, ok := <-stdinLines:
			if !ok {
				// stdin EOF — keep watching SSE until ctx or SSE closes.
				stdinLines = nil // nil disables this case in future selects
				continue
			}
			if line.err != nil {
				fmt.Fprintln(os.Stderr, "stdin error:", line.err)
				stdinLines = nil
				continue
			}
			text := strings.TrimSpace(line.text)
			if text == "" {
				continue
			}
			var filePath, textArg string
			if after, ok := strings.CutPrefix(text, ":file "); ok {
				filePath = strings.TrimSpace(after)
			} else {
				textArg = text
			}
			if pushErr := cmdPush(ctx, serverURL, uuid, passphrase, deviceName, filePath, textArg); pushErr != nil {
				fmt.Fprintln(os.Stderr, "send error:", pushErr)
			}
		}
	}
}

// handleMessage decrypts one SSE/pull message and prints/saves it.
func handleMessage(b64blob, pushedAt, uuid, passphrase string, seen map[string]bool) {
	blobBytes, err := base64.StdEncoding.DecodeString(b64blob)
	if err != nil {
		return
	}
	plain, err := channel.DecryptBlob(blobBytes, passphrase, uuid)
	if err != nil {
		return
	}
	env, err := parseEnvelope(plain)
	if err != nil {
		return
	}

	// Deduplicate by msg_id.
	idKey := base64.RawStdEncoding.EncodeToString(env.msgID)
	if seen[idKey] {
		return
	}
	seen[idKey] = true
	if len(seen) > 200 {
		// Trim oldest entries to cap memory (simple approach: clear half).
		for k := range seen {
			delete(seen, k)
			if len(seen) <= 100 {
				break
			}
		}
	}

	ts := pushedAt
	if t, err := time.Parse(time.RFC3339, pushedAt); err == nil {
		ts = t.Local().Format("2006-01-02 15:04:05")
	}

	name := env.senderName
	if name == "" {
		name = "anon"
	}
	sender := "(" + name + ") "

	switch env.msgType {
	case msgTypeText:
		fmt.Printf("%s %s%s\n", ts, sender, string(env.payload))

	case msgTypeFile:
		if len(env.payload) < 2 {
			return
		}
		fnLen := int(binary.BigEndian.Uint16(env.payload[0:2]))
		if 2+fnLen > len(env.payload) {
			return
		}
		rawName := string(env.payload[2 : 2+fnLen])
		filename := util.SanitizeFilename(filepath.Base(rawName))
		if filename == "" {
			filename = "file"
		}
		fileData := env.payload[2+fnLen:]
		savePath := uniqueFilename(filename)
		if err := os.WriteFile(savePath, fileData, 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "save %s: %v\n", savePath, err)
			return
		}
		fmt.Printf("%s %ssaved file: %s\n", ts, sender, savePath)
	}
}

type envelope struct {
	msgID      []byte
	msgType    byte
	senderName string
	payload    []byte
}

func parseEnvelope(plain []byte) (*envelope, error) {
	if len(plain) < channel.MinInnerSize {
		return nil, fmt.Errorf("envelope too short")
	}
	msgID := plain[0:16]
	typ := plain[16]
	if typ != msgTypeText && typ != msgTypeFile {
		return nil, fmt.Errorf("unknown type: 0x%02x", typ)
	}
	nameLen := int(plain[17])
	if nameLen > 32 || 18+nameLen > len(plain) {
		return nil, fmt.Errorf("invalid name len")
	}
	rawName := plain[18 : 18+nameLen]
	senderName := util.StripControl(string(rawName))

	payload := plain[18+nameLen:]
	return &envelope{
		msgID:      msgID,
		msgType:    typ,
		senderName: senderName,
		payload:    payload,
	}, nil
}

func buildEnvelope(msgType byte, senderName string, payload []byte) ([]byte, error) {
	msgID := make([]byte, 16)
	if _, err := rand.Read(msgID); err != nil {
		return nil, err
	}
	nameBytes := []byte(util.TruncateUTF8(senderName, 32))
	env := make([]byte, 0, 16+1+1+len(nameBytes)+len(payload))
	env = append(env, msgID...)
	env = append(env, msgType)
	env = append(env, byte(len(nameBytes)))
	env = append(env, nameBytes...)
	env = append(env, payload...)
	return env, nil
}

func readPassphrase(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	// term.ReadPassword disables echo so the passphrase is not visible.
	// Falls back to plain stdin read when stdin is not a terminal (e.g. pipes).
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr) // newline after hidden input
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// uniqueFilename appends _1, _2, … to avoid overwriting existing files.
func uniqueFilename(name string) string {
	if _, err := os.Stat(name); os.IsNotExist(err) {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return name
}
