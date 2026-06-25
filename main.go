package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	_ "embed"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed static/index.html
var indexHTML []byte

const (
	maxFileSize = 50 << 20 // 50 MB
	sgnBinary   = "/usr/local/bin/sgn"
)

type encodeResponse struct {
	Success  bool   `json:"success"`
	Log      string `json:"log"`
	File     string `json:"file,omitempty"`
	Filename string `json:"filename,omitempty"`
	InSize   int64  `json:"inSize,omitempty"`
	OutSize  int    `json:"outSize,omitempty"`
	Error    string `json:"error,omitempty"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/encode", handleEncode)
	mux.HandleFunc("/health", handleHealth)

	addr := "0.0.0.0:" + port
	log.Printf("[sgn-web] Listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[sgn-web] Fatal: %v", err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("ok"))
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func handleEncode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)
	if err := r.ParseMultipartForm(maxFileSize); err != nil {
		writeJSON(w, http.StatusBadRequest, encodeResponse{
			Success: false,
			Error:   "Failed to parse form: " + err.Error(),
		})
		return
	}

	file, header, err := r.FormFile("payload")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, encodeResponse{
			Success: false,
			Error:   "No payload file provided",
		})
		return
	}
	defer file.Close()

	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "sgn-web-*")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, encodeResponse{
			Success: false,
			Error:   "Server error: " + err.Error(),
		})
		return
	}
	defer os.RemoveAll(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.bin")
	outputPath := filepath.Join(tmpDir, "output.bin")

	// Write uploaded file to disk
	f, err := os.Create(inputPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, encodeResponse{
			Success: false,
			Error:   "Server error: " + err.Error(),
		})
		return
	}
	inSize, err := io.Copy(f, file)
	f.Close()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, encodeResponse{
			Success: false,
			Error:   "Failed to save payload: " + err.Error(),
		})
		return
	}

	// Build SGN arguments
	args := []string{"-i", inputPath, "-o", outputPath}

	// Architecture is fixed to 64-bit
	args = append(args, "-a", "64")

	if enc := r.FormValue("enc"); enc != "" {
		if n, err2 := strconv.Atoi(enc); err2 == nil && n >= 1 && n <= 20 {
			args = append(args, "-c", enc)
		}
	}

	if max := r.FormValue("max"); max != "" {
		if n, err2 := strconv.Atoi(max); err2 == nil && n >= 1 && n <= 255 {
			args = append(args, "-M", max)
		}
	}

	if r.FormValue("safe") == "1" {
		args = append(args, "-S")
	}
	if r.FormValue("plain") == "1" {
		args = append(args, "--plain")
	}
	if r.FormValue("ascii") == "1" {
		args = append(args, "--ascii")
	}
	if bc := r.FormValue("badchars"); bc != "" {
		args = append(args, "--badchars", bc)
	}

	// Timeout: default 5 min; override with SGN_TIMEOUT env (e.g. "15m")
	timeout := 5 * time.Minute
	if ts := os.Getenv("SGN_TIMEOUT"); ts != "" {
		if d, err2 := time.ParseDuration(ts); err2 == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, sgnBinary, args...)
	out, runErr := cmd.CombinedOutput()
	logOutput := string(out)

	if ctx.Err() == context.DeadlineExceeded {
		writeJSON(w, http.StatusRequestTimeout, encodeResponse{
			Success: false,
			Error:   "Encoding timed out after " + timeout.String() + " — ASCII mode may require SGN_TIMEOUT=30m",
			Log:     logOutput,
		})
		return
	}

	if runErr != nil {
		writeJSON(w, http.StatusInternalServerError, encodeResponse{
			Success: false,
			Error:   "SGN encoding failed: " + runErr.Error(),
			Log:     logOutput,
		})
		return
	}

	encoded, err := os.ReadFile(outputPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, encodeResponse{
			Success: false,
			Error:   "Failed to read encoded output: " + err.Error(),
			Log:     logOutput,
		})
		return
	}

	// Build download filename
	origName := header.Filename
	if origName == "" {
		origName = "payload.bin"
	}
	ext := filepath.Ext(origName)
	base := strings.TrimSuffix(origName, ext)
	outFilename := base + "_sgn" + ext

	writeJSON(w, http.StatusOK, encodeResponse{
		Success:  true,
		Log:      logOutput,
		File:     base64.StdEncoding.EncodeToString(encoded),
		Filename: outFilename,
		InSize:   inSize,
		OutSize:  len(encoded),
	})
}
