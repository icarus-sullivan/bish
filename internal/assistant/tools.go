package assistant

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"  // registers gif with image.Decode, used below
	_ "image/jpeg" // registers jpeg (incl. CMYK) with image.Decode, used below
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/csullivan/bish/internal/search"
)

// maxToolReadSize caps read_file's text path — a local model's context
// window is far smaller than an editor buffer, so this is much tighter than
// app.go's maxEditorFileSize.
const maxToolReadSize = 512 * 1024

// maxToolImageSize caps read_file's image path — vision models pay for
// every image in context too, just via a different (larger, roughly
// fixed-per-image) budget than raw text tokens.
const maxToolImageSize = 8 * 1024 * 1024

// imageExts/videoExts mirror app.go's mediaExts subsets; duplicated rather
// than imported since internal/app already imports internal/assistant (a
// cycle the other way isn't possible) and these are small literals.
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".webp": true,
}

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".webm": true, ".mkv": true, ".avi": true,
}

// audioExts is checked only to give a clear "not supported" tool error
// instead of silently mangling the file as binary — see the read_file case.
var audioExts = map[string]bool{
	".mp3": true, ".wav": true, ".m4a": true, ".flac": true, ".ogg": true, ".aac": true,
}

// toolResult is what a tool call produces: either text (the common case) or
// an image the model should see directly, attached to the tool-result
// message's "images" field for vision-capable models.
type toolResult struct {
	Text  string
	Image string // base64-encoded, no data-URI prefix (Ollama's expected format)
}

// toolDef is one function the local model can call. Mutating tools are the
// ones gated behind plan-mode approval in ollamaBackend.
type toolDef struct {
	Name        string
	Description string
	Params      map[string]toolParam
	Required    []string
	Mutating    bool
}

type toolParam struct {
	Type        string
	Description string
}

var tools = []toolDef{
	{
		Name: "read_file",
		Description: "Read the full contents of a text file, view an image file (png/jpg/gif/webp/bmp) directly, " +
			"or get a single still-frame preview of a video file (mp4/mov/webm/mkv/avi — no motion or audio). " +
			"Audio files are not supported.",
		Params: map[string]toolParam{
			"path": {"string", "File path — relative to the project root, or an absolute path inside it."},
		},
		Required: []string{"path"},
	},
	{
		Name:        "list_files",
		Description: "Recursively list file paths under a directory.",
		Params: map[string]toolParam{
			"path": {"string", "Directory path — relative to the project root (\".\" for the whole project), or an absolute path inside it."},
		},
		Required: []string{"path"},
	},
	{
		Name:        "search_files",
		Description: "Search for a plain-text query across files under a directory. Returns matching file:line: text lines.",
		Params: map[string]toolParam{
			"path":  {"string", "Directory to search under — relative to the project root, or an absolute path inside it."},
			"query": {"string", "Text to search for."},
		},
		Required: []string{"path", "query"},
	},
	{
		Name:        "write_file",
		Description: "Overwrite a file with new full contents, creating it (and any parent directories) if needed.",
		Params: map[string]toolParam{
			"path":    {"string", "File path — relative to the project root, or an absolute path inside it."},
			"content": {"string", "The complete new contents of the file."},
		},
		Required: []string{"path", "content"},
		Mutating: true,
	},
	{
		Name:        "run_shell",
		Description: "Run a shell command in the project root and return its stdout, stderr, and exit code.",
		Params: map[string]toolParam{
			"command": {"string", "The shell command to run."},
		},
		Required: []string{"command"},
		Mutating: true,
	},
}

func isMutating(name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return t.Mutating
		}
	}
	return false
}

// toolSchemas renders the tool set as Ollama/OpenAI-style function definitions
// for the request's "tools" field.
func toolSchemas() []map[string]any {
	out := make([]map[string]any, len(tools))
	for i, t := range tools {
		props := map[string]any{}
		for k, p := range t.Params {
			props[k] = map[string]any{"type": p.Type, "description": p.Description}
		}
		out[i] = map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters": map[string]any{
					"type":       "object",
					"properties": props,
					"required":   t.Required,
				},
			},
		}
	}
	return out
}

// resolvePath confines a tool-supplied path to root, accepting either a
// relative path or an absolute one that's already inside root (models
// happily emit absolute paths when they've seen one earlier in the
// conversation — e.g. from a prior tool result). Only an actual ".."-escape
// or an absolute path outside root is rejected. This is the only safety net
// here — there is no broader sandboxing anywhere in the codebase, and a
// local model executing tool calls unattended needs at least a
// project-root boundary.
func resolvePath(root, path string) (string, error) {
	full := path
	if !filepath.IsAbs(path) {
		full = filepath.Join(root, path)
	}
	full = filepath.Clean(full)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside the project root", path)
	}
	return full, nil
}

// runTool executes one tool call and returns its result (or an error, which
// the caller feeds back to the model as the tool result rather than failing
// the whole turn). hasVision reflects whether the configured model actually
// reports Ollama's "vision" capability (see GetCapabilities) — a non-vision
// model gets a clear refusal instead of an image it can't use.
func runTool(root, name string, args map[string]any, hasVision bool) (toolResult, error) {
	str := func(k string) string {
		s, _ := args[k].(string)
		return s
	}
	switch name {
	case "read_file":
		full, err := resolvePath(root, str("path"))
		if err != nil {
			return toolResult{}, err
		}
		ext := strings.ToLower(filepath.Ext(full))

		if imageExts[ext] || videoExts[ext] {
			if !hasVision {
				return toolResult{}, fmt.Errorf("this model has no vision capability — it cannot view images or video frames")
			}
			img, err := readImageOrFrame(full, ext)
			if err != nil {
				return toolResult{}, err
			}
			return img, nil
		}
		if audioExts[ext] {
			return toolResult{}, fmt.Errorf("audio input isn't supported: Ollama's chat API has no documented audio channel (unlike images), so this can't be reliably delivered to the model")
		}

		if fi, err := os.Stat(full); err == nil && fi.Size() > maxToolReadSize {
			return toolResult{}, fmt.Errorf("file too large to read (%.1f MB, limit %d MB)",
				float64(fi.Size())/(1024*1024), maxToolReadSize/(1024*1024))
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return toolResult{}, err
		}
		// git heuristic: null byte in first 8000 bytes = binary. Feeding raw
		// binary bytes to the model as "text" bloats the request and tends to
		// stall small local models entirely rather than erroring cleanly.
		if bytes.IndexByte(data[:min(len(data), 8000)], 0) != -1 {
			return toolResult{}, fmt.Errorf("%q is a binary file of an unrecognized type — read_file only handles text, images, and video previews", str("path"))
		}
		return toolResult{Text: string(data)}, nil

	case "list_files":
		full, err := resolvePath(root, str("path"))
		if err != nil {
			return toolResult{}, err
		}
		files := search.AllFiles(full)
		rel := make([]string, len(files))
		for i, f := range files {
			r, err := filepath.Rel(root, f)
			if err != nil {
				r = f
			}
			rel[i] = r
		}
		return toolResult{Text: strings.Join(rel, "\n")}, nil

	case "search_files":
		full, err := resolvePath(root, str("path"))
		if err != nil {
			return toolResult{}, err
		}
		results := search.Search(full, str("query"), false, false, false)
		var b strings.Builder
		for _, r := range results {
			rel, err := filepath.Rel(root, r.File)
			if err != nil {
				rel = r.File
			}
			fmt.Fprintf(&b, "%s:%d: %s\n", rel, r.Line, r.Text)
		}
		return toolResult{Text: b.String()}, nil

	case "write_file":
		full, err := resolvePath(root, str("path"))
		if err != nil {
			return toolResult{}, err
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return toolResult{}, err
		}
		if err := os.WriteFile(full, []byte(str("content")), 0o644); err != nil {
			return toolResult{}, err
		}
		return toolResult{Text: "ok"}, nil

	case "run_shell":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", str("command"))
		cmd.Dir = root
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		exitCode := 0
		if runErr := cmd.Run(); runErr != nil {
			if ee, ok := runErr.(*exec.ExitError); ok {
				exitCode = ee.ExitCode()
			} else {
				return toolResult{}, runErr
			}
		}
		return toolResult{Text: fmt.Sprintf("exit code: %d\nstdout:\n%s\nstderr:\n%s", exitCode, stdout.String(), stderr.String())}, nil

	default:
		return toolResult{}, fmt.Errorf("unknown tool %q", name)
	}
}

// readImageOrFrame returns a static image's bytes as-is, or — for a video —
// a single extracted frame via ffmpeg, since Ollama's API only ever accepts
// still images, never a video file itself.
func readImageOrFrame(full, ext string) (toolResult, error) {
	if imageExts[ext] {
		fi, err := os.Stat(full)
		if err != nil {
			return toolResult{}, err
		}
		if fi.Size() > maxToolImageSize {
			return toolResult{}, fmt.Errorf("image too large to attach (%.1f MB, limit %d MB)",
				float64(fi.Size())/(1024*1024), maxToolImageSize/(1024*1024))
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return toolResult{}, err
		}
		// Re-encode through Go's stdlib decoder when we can. Ollama's own
		// image loader (stb_image, via llama.cpp) is known to choke on
		// things Go's decoder handles fine — CMYK JPEGs being the classic
		// case — producing an opaque "Failed to load image or audio file"
		// 400 with no indication it was the source encoding at fault.
		// Round-tripping to a clean baseline PNG sidesteps that. bmp/webp
		// have no stdlib decoder, so those still pass through as-is.
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" {
			img, _, decodeErr := image.Decode(bytes.NewReader(data))
			if decodeErr != nil {
				return toolResult{}, fmt.Errorf("could not decode image %q: %w", full, decodeErr)
			}
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				return toolResult{}, fmt.Errorf("could not re-encode image %q: %w", full, err)
			}
			if buf.Len() > maxToolImageSize {
				return toolResult{}, fmt.Errorf("re-encoded image too large to attach (%.1f MB, limit %d MB)",
					float64(buf.Len())/(1024*1024), maxToolImageSize/(1024*1024))
			}
			data = buf.Bytes()
		}
		return toolResult{Image: base64.StdEncoding.EncodeToString(data)}, nil
	}

	// video: extract one frame with ffmpeg rather than pretending a video
	// file can go straight into Ollama's "images" field.
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return toolResult{}, fmt.Errorf("ffmpeg not found on PATH — needed to extract a preview frame from video files")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-v", "error", "-i", full, "-frames:v", "1", "-f", "image2", "-vcodec", "png", "pipe:1")
	var frame, stderr bytes.Buffer
	cmd.Stdout = &frame
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return toolResult{}, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if frame.Len() == 0 {
		return toolResult{}, fmt.Errorf("ffmpeg produced no frame for %q", full)
	}
	if frame.Len() > maxToolImageSize {
		return toolResult{}, fmt.Errorf("extracted video frame too large to attach (%.1f MB, limit %d MB)",
			float64(frame.Len())/(1024*1024), maxToolImageSize/(1024*1024))
	}
	return toolResult{
		Image: base64.StdEncoding.EncodeToString(frame.Bytes()),
		Text:  "This is a single still frame extracted from the video — no motion or audio is available to me.",
	}, nil
}
