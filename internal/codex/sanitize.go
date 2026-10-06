package codex

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	idRe    = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
	modelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/\[\]-]{0,127}$`)

	sandboxModes = map[string]bool{"read-only": true, "workspace-write": true, "danger-full-access": true}
	approvals    = map[string]bool{"untrusted": true, "on-request": true, "never": true}
	efforts      = map[string]bool{"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}
	summaries    = map[string]bool{"auto": true, "concise": true, "detailed": true, "none": true}
)

// methods the panel may call; anything else (fs/*, config writes, process
// spawning, plugin installs, login flows, …) is refused outright.
var allowedMethods = map[string]bool{
	"thread/start": true, "thread/resume": true, "thread/fork": true, "thread/read": true,
	"thread/list": true, "thread/name/set": true, "thread/compact/start": true, "thread/revert": true,
	"turn/start": true, "turn/steer": true, "turn/interrupt": true,
	"model/list": true, "mcpServerStatus/list": true, "account/read": true, "account/rateLimits/read": true,
	"account/login/start": true, "account/login/cancel": true,
}

// sanitizeParams rebuilds the params object for method from known-safe
// fields only. cwd is always the session's project root.
func sanitizeParams(method string, in map[string]any, root string) (map[string]any, error) {
	if !allowedMethods[method] {
		return nil, fmt.Errorf("codex: method %q not allowed", method)
	}
	out := map[string]any{}
	str := func(k string) (string, bool) { v, ok := in[k].(string); return v, ok && v != "" }
	needID := func(k string) error {
		v, ok := str(k)
		if !ok || !idRe.MatchString(v) {
			return fmt.Errorf("codex: %s: invalid %s", method, k)
		}
		out[k] = v
		return nil
	}
	optID := func(k string) error {
		if _, ok := in[k]; !ok {
			return nil
		}
		return needID(k)
	}
	enum := func(k string, set map[string]bool) error {
		v, ok := str(k)
		if !ok {
			return nil
		}
		if !set[v] {
			return fmt.Errorf("codex: %s: invalid %s %q", method, k, v)
		}
		out[k] = v
		return nil
	}
	model := func() error {
		v, ok := str("model")
		if !ok {
			return nil
		}
		if !modelRe.MatchString(v) {
			return fmt.Errorf("codex: invalid model %q", v)
		}
		out["model"] = v
		return nil
	}
	var errs []error
	add := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	switch method {
	case "thread/start":
		out["cwd"] = root
		add(model())
		add(enum("sandbox", sandboxModes))
		add(enum("approvalPolicy", approvals))
	case "thread/resume", "thread/fork":
		add(needID("threadId"))
		out["cwd"] = root
		add(model())
		add(enum("sandbox", sandboxModes))
		add(enum("approvalPolicy", approvals))
		if method == "thread/fork" {
			add(optID("lastTurnId"))
		}
	case "thread/read":
		add(needID("threadId"))
		out["includeTurns"] = in["includeTurns"] == true
	case "thread/list":
		out["cwd"] = root // only this project's threads
		if v, ok := in["limit"].(float64); ok && v > 0 && v <= 200 {
			out["limit"] = int(v)
		}
		add(optID("cursor"))
		if v, ok := str("searchTerm"); ok && len(v) <= 200 {
			out["searchTerm"] = v
		}
	case "thread/name/set":
		add(needID("threadId"))
		name, _ := str("name")
		if len(name) > 200 {
			name = name[:200]
		}
		out["name"] = name
	case "thread/compact/start":
		add(needID("threadId"))
	case "thread/revert":
		add(needID("threadId"))
		add(needID("beforeTurnId"))
	case "turn/interrupt":
		add(needID("threadId"))
		add(needID("turnId"))
	case "turn/start", "turn/steer":
		add(needID("threadId"))
		input, err := sanitizeInput(in["input"], root)
		add(err)
		out["input"] = input
		if method == "turn/steer" {
			add(needID("expectedTurnId"))
			break
		}
		add(model())
		add(enum("effort", efforts))
		add(enum("summary", summaries))
		add(enum("approvalPolicy", approvals))
		if sp, ok := in["sandboxPolicy"].(map[string]any); ok {
			pol, err := sanitizeSandboxPolicy(sp)
			add(err)
			if pol != nil {
				out["sandboxPolicy"] = pol
			}
		}
	case "model/list":
		out["includeHidden"] = false
	case "mcpServerStatus/list":
		if v, ok := str("threadId"); ok && idRe.MatchString(v) {
			out["threadId"] = v
		}
	case "account/read":
		out["refreshToken"] = false
	case "account/rateLimits/read":
		return nil, nil
	case "account/login/start":
		// browser sign-in or an API key; token-injection and Bedrock variants are refused
		switch in["type"] {
		case "chatgpt":
			out["type"] = "chatgpt"
		case "apiKey":
			k, _ := str("apiKey")
			if k == "" || len(k) > 512 || strings.ContainsAny(k, " \t\r\n") {
				return nil, fmt.Errorf("codex: invalid API key")
			}
			out["type"] = "apiKey"
			out["apiKey"] = k
		default:
			return nil, fmt.Errorf("codex: unsupported sign-in type")
		}
	case "account/login/cancel":
		add(needID("loginId"))
	}
	if len(errs) > 0 {
		return nil, errs[0]
	}
	return out, nil
}

// sanitizeInput keeps text, local images, and file mentions. Local images
// must be absolute paths to image files; mentions must stay inside root.
func sanitizeInput(v any, root string) ([]map[string]any, error) {
	items, ok := v.([]any)
	if !ok || len(items) == 0 || len(items) > 64 {
		return nil, fmt.Errorf("codex: input must be a non-empty list")
	}
	var out []map[string]any
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("codex: bad input item")
		}
		switch m["type"] {
		case "text":
			t, _ := m["text"].(string)
			out = append(out, map[string]any{"type": "text", "text": t})
		case "localImage":
			p, _ := m["path"].(string)
			ext := strings.ToLower(filepath.Ext(p))
			if !filepath.IsAbs(p) || (ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" && ext != ".webp") {
				return nil, fmt.Errorf("codex: image must be an absolute path to an image file")
			}
			out = append(out, map[string]any{"type": "localImage", "path": filepath.Clean(p)})
		case "mention":
			p, _ := m["path"].(string)
			name, _ := m["name"].(string)
			clean := filepath.Clean(p)
			if !filepath.IsAbs(clean) || (clean != root && !strings.HasPrefix(clean, root+string(filepath.Separator))) {
				return nil, fmt.Errorf("codex: mentioned file must be inside the project")
			}
			out = append(out, map[string]any{"type": "mention", "name": name, "path": clean})
		default:
			return nil, fmt.Errorf("codex: unsupported input type %v", m["type"])
		}
	}
	return out, nil
}

// sanitizeSandboxPolicy accepts the three panel presets only; writable roots
// can't be widened from the frontend.
func sanitizeSandboxPolicy(p map[string]any) (map[string]any, error) {
	net := p["networkAccess"] == true
	switch p["type"] {
	case "readOnly":
		return map[string]any{"type": "readOnly", "networkAccess": net}, nil
	case "workspaceWrite":
		return map[string]any{"type": "workspaceWrite", "networkAccess": net}, nil
	case "dangerFullAccess":
		return map[string]any{"type": "dangerFullAccess"}, nil
	}
	return nil, fmt.Errorf("codex: invalid sandbox policy")
}

var (
	cmdDecisions  = map[string]bool{"accept": true, "acceptForSession": true, "decline": true, "cancel": true}
	fileDecisions = map[string]bool{"accept": true, "acceptForSession": true, "decline": true, "cancel": true}
)

// sanitizeResponse shapes the answer to a server request. Exec-policy and
// network-policy amendments (which persist rules) aren't offered.
func sanitizeResponse(method string, r map[string]any) (map[string]any, error) {
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		d, _ := r["decision"].(string)
		set := cmdDecisions
		if method == "item/fileChange/requestApproval" {
			set = fileDecisions
		}
		if !set[d] {
			return nil, fmt.Errorf("codex: invalid decision %q", d)
		}
		return map[string]any{"decision": d}, nil
	case "item/tool/requestUserInput":
		raw, _ := r["answers"].(map[string]any)
		answers := map[string]any{}
		for k, v := range raw {
			if !idRe.MatchString(k) {
				continue
			}
			var list []string
			if m, ok := v.(map[string]any); ok {
				if arr, ok := m["answers"].([]any); ok {
					for _, a := range arr {
						if s, ok := a.(string); ok {
							list = append(list, s)
						}
					}
				}
			}
			answers[k] = map[string]any{"answers": list}
		}
		return map[string]any{"answers": answers}, nil
	}
	return nil, fmt.Errorf("codex: cannot answer %s", method)
}
