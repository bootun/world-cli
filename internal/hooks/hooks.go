package hooks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	TargetAuto        = "auto"
	TargetAll         = "all"
	TargetAntigravity = "antigravity"
	TargetClaude      = "claude"
	TargetCodex       = "codex"

	ScopeUser    = "user"
	ScopeProject = "project"

	DialectAuto        = "auto"
	DialectAntigravity = "antigravity"
	DialectClaudeCodex = "claude_codex"

	AntigravityHookName = "world-machine"
	DefaultCommandBin   = "world"
	DefaultTimeoutSec   = 10
)

const (
	SessionStartPrompt = "World Machine memory (`world`) is active. " +
		"1) Before starting any non-trivial task, question, analysis, or planning, run `world recall \"<relevant topic, person, project, or context>\" --max-entities 8 --max-events 10 --json` first to check for helpful prior context or historical state. " +
		"2) During the conversation, whenever durable preferences, noteworthy events, state changes, decisions, commitments, people/relationship updates, factual corrections, or settled conclusions surface, record them quietly with `world remember \"<self-contained statement>\" --source agent --json` (never store secrets or ephemeral chitchat)."

	StopReviewPrompt = "Before finishing this turn, briefly review what happened in this turn: did any durable preference, noteworthy event, state change, decision, commitment, relationship/ownership update, factual correction, or settled conclusion emerge that has not yet been saved? " +
		"If YES: quietly run `world remember \"<self-contained statement>\" --source agent --json` (one call per independent fact/event, preserving original relative time phrases, never storing secrets). " +
		"If NO (pure chitchat, ephemeral instruction, unsettled draft, or already recorded): do nothing and finish immediately without mentioning this check."
)

var allTargets = []string{TargetAntigravity, TargetClaude, TargetCodex}

type TargetStatus struct {
	Target                string `json:"target"`
	Scope                 string `json:"scope"`
	Detected              bool   `json:"detected"`
	ConfigPath            string `json:"configPath"`
	SessionStartInstalled bool   `json:"sessionStartInstalled"`
	StopInstalled         bool   `json:"stopInstalled"`
	Changed               bool   `json:"changed,omitempty"`
}

type ManageOptions struct {
	Target     string
	Scope      string
	CommandBin string
	HomeDir    string
	WorkDir    string
}

func NormalizeScope(scope string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(scope))
	if s == "" || s == ScopeUser {
		return ScopeUser, nil
	}
	if s == ScopeProject {
		return ScopeProject, nil
	}
	return "", fmt.Errorf("invalid --scope %q (expected user or project)", scope)
}

func ResolveTargets(targetArg, scope, homeDir, workDir string) ([]string, error) {
	raw := strings.ToLower(strings.TrimSpace(targetArg))
	if raw == "" || raw == TargetAuto {
		var detected []string
		for _, t := range allTargets {
			if isTargetDetected(t, scope, homeDir, workDir) {
				detected = append(detected, t)
			}
		}
		return detected, nil
	}
	if raw == TargetAll {
		return append([]string(nil), allTargets...), nil
	}

	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		switch p {
		case TargetAntigravity, "agy", "gemini":
			p = TargetAntigravity
		case TargetClaude, "claude-code":
			p = TargetClaude
		case TargetCodex, "codex-cli":
			p = TargetCodex
		default:
			return nil, fmt.Errorf("unknown --target %q (expected auto, all, antigravity, claude, or codex)", p)
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty --target value")
	}
	return out, nil
}

func ConfigPath(target, scope, homeDir, workDir string) (string, error) {
	switch target {
	case TargetAntigravity:
		if scope == ScopeProject {
			return filepath.Join(workDir, ".agents", "hooks.json"), nil
		}
		return filepath.Join(homeDir, ".gemini", "config", "hooks.json"), nil
	case TargetClaude:
		if scope == ScopeProject {
			return filepath.Join(workDir, ".claude", "settings.json"), nil
		}
		return filepath.Join(homeDir, ".claude", "settings.json"), nil
	case TargetCodex:
		if scope == ScopeProject {
			return filepath.Join(workDir, ".codex", "hooks.json"), nil
		}
		return filepath.Join(homeDir, ".codex", "hooks.json"), nil
	default:
		return "", fmt.Errorf("unsupported target %q", target)
	}
}

func isTargetDetected(target, scope, homeDir, workDir string) bool {
	if scope == ScopeProject {
		switch target {
		case TargetAntigravity:
			return dirExists(filepath.Join(workDir, ".agents")) || dirExists(filepath.Join(workDir, ".agent"))
		case TargetClaude:
			return dirExists(filepath.Join(workDir, ".claude"))
		case TargetCodex:
			return dirExists(filepath.Join(workDir, ".codex"))
		}
		return false
	}
	switch target {
	case TargetAntigravity:
		return dirExists(filepath.Join(homeDir, ".gemini", "config")) || dirExists(filepath.Join(homeDir, ".gemini", "antigravity"))
	case TargetClaude:
		return dirExists(filepath.Join(homeDir, ".claude"))
	case TargetCodex:
		return dirExists(filepath.Join(homeDir, ".codex"))
	}
	return false
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func ResolveCommandBin(explicit string, executable func() (string, error), lookPath func(string) (string, error)) string {
	if v := strings.TrimSpace(explicit); v != "" {
		return v
	}
	if executable != nil {
		if exe, err := executable(); err == nil {
			exe = filepath.Clean(strings.TrimSpace(exe))
			base := strings.ToLower(filepath.Base(exe))
			if exe != "" && (base == "world" || base == "world.exe") {
				return exe
			}
		}
	}
	if lookPath != nil {
		if found, err := lookPath(DefaultCommandBin); err == nil && strings.TrimSpace(found) != "" {
			return filepath.Clean(found)
		}
	}
	return DefaultCommandBin
}

func commandForEvent(bin, event string) string {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		bin = DefaultCommandBin
	}
	if strings.ContainsAny(bin, " \t") && !strings.HasPrefix(bin, "\"") {
		bin = `"` + bin + `"`
	}
	return bin + " hook " + event
}

func isWorldHookCommand(cmd, event string) bool {
	c := strings.ToLower(strings.TrimSpace(cmd))
	if !strings.Contains(c, "hook "+strings.ToLower(event)) {
		return false
	}
	return strings.Contains(c, "world ") ||
		strings.Contains(c, "world\"") ||
		strings.Contains(c, "/world") ||
		strings.Contains(c, `\world`) ||
		strings.Contains(c, "world.exe")
}

func Install(opts ManageOptions) ([]TargetStatus, error) {
	scope, err := NormalizeScope(opts.Scope)
	if err != nil {
		return nil, err
	}
	targets, err := ResolveTargets(opts.Target, scope, opts.HomeDir, opts.WorkDir)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}

	sessionCmd := commandForEvent(opts.CommandBin, "session-start")
	stopCmd := commandForEvent(opts.CommandBin, "stop")

	results := make([]TargetStatus, 0, len(targets))
	for _, target := range targets {
		cfgPath, err := ConfigPath(target, scope, opts.HomeDir, opts.WorkDir)
		if err != nil {
			return nil, err
		}
		changed, err := installIntoFile(target, cfgPath, sessionCmd, stopCmd)
		if err != nil {
			return nil, fmt.Errorf("install %s hooks (%s): %w", target, cfgPath, err)
		}
		results = append(results, TargetStatus{
			Target:                target,
			Scope:                 scope,
			Detected:              isTargetDetected(target, scope, opts.HomeDir, opts.WorkDir),
			ConfigPath:            cfgPath,
			SessionStartInstalled: true,
			StopInstalled:         true,
			Changed:               changed,
		})
	}
	return results, nil
}

func Uninstall(opts ManageOptions) ([]TargetStatus, error) {
	scope, err := NormalizeScope(opts.Scope)
	if err != nil {
		return nil, err
	}
	targetArg := strings.TrimSpace(opts.Target)
	if targetArg == "" || strings.EqualFold(targetArg, TargetAuto) {
		targetArg = TargetAll
	}
	targets, err := ResolveTargets(targetArg, scope, opts.HomeDir, opts.WorkDir)
	if err != nil {
		return nil, err
	}

	results := make([]TargetStatus, 0, len(targets))
	for _, target := range targets {
		cfgPath, err := ConfigPath(target, scope, opts.HomeDir, opts.WorkDir)
		if err != nil {
			return nil, err
		}
		changed, err := uninstallFromFile(target, cfgPath)
		if err != nil {
			return nil, fmt.Errorf("uninstall %s hooks (%s): %w", target, cfgPath, err)
		}
		if changed || strings.ToLower(strings.TrimSpace(opts.Target)) != TargetAuto && strings.TrimSpace(opts.Target) != "" {
			results = append(results, TargetStatus{
				Target:                target,
				Scope:                 scope,
				Detected:              isTargetDetected(target, scope, opts.HomeDir, opts.WorkDir),
				ConfigPath:            cfgPath,
				SessionStartInstalled: false,
				StopInstalled:         false,
				Changed:               changed,
			})
		}
	}
	return results, nil
}

func Status(opts ManageOptions) ([]TargetStatus, error) {
	scope, err := NormalizeScope(opts.Scope)
	if err != nil {
		return nil, err
	}
	targetArg := strings.TrimSpace(opts.Target)
	if targetArg == "" || strings.EqualFold(targetArg, TargetAuto) {
		targetArg = TargetAll
	}
	targets, err := ResolveTargets(targetArg, scope, opts.HomeDir, opts.WorkDir)
	if err != nil {
		return nil, err
	}

	results := make([]TargetStatus, 0, len(targets))
	for _, target := range targets {
		cfgPath, err := ConfigPath(target, scope, opts.HomeDir, opts.WorkDir)
		if err != nil {
			return nil, err
		}
		ss, st, err := inspectFile(target, cfgPath)
		if err != nil {
			return nil, fmt.Errorf("inspect %s hooks (%s): %w", target, cfgPath, err)
		}
		results = append(results, TargetStatus{
			Target:                target,
			Scope:                 scope,
			Detected:              isTargetDetected(target, scope, opts.HomeDir, opts.WorkDir),
			ConfigPath:            cfgPath,
			SessionStartInstalled: ss,
			StopInstalled:         st,
		})
	}
	return results, nil
}

func readJSONMap(path string) (map[string]any, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]any{}, true, nil
	}
	var root map[string]any
	if err := json.Unmarshal(trimmed, &root); err != nil {
		return nil, true, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}
	if root == nil {
		root = map[string]any{}
	}
	return root, true, nil
}

func writeJSONAtomic(path string, root map[string]any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(dir, ".world-hooks-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func installIntoFile(target, path, sessionCmd, stopCmd string) (bool, error) {
	root, existed, err := readJSONMap(path)
	if err != nil {
		return false, err
	}
	before, _ := json.Marshal(root)

	switch target {
	case TargetAntigravity:
		root[AntigravityHookName] = map[string]any{
			"SessionStart": []any{
				map[string]any{
					"type":    "command",
					"command": sessionCmd,
					"timeout": DefaultTimeoutSec,
				},
			},
			"Stop": []any{
				map[string]any{
					"type":    "command",
					"command": stopCmd,
					"timeout": DefaultTimeoutSec,
				},
			},
		}
	case TargetClaude, TargetCodex:
		hooksMap, _ := root["hooks"].(map[string]any)
		if hooksMap == nil {
			hooksMap = map[string]any{}
		}
		hooksMap["SessionStart"] = upsertClaudeCodexEvent(hooksMap["SessionStart"], "session-start", sessionCmd)
		hooksMap["Stop"] = upsertClaudeCodexEvent(hooksMap["Stop"], "stop", stopCmd)
		root["hooks"] = hooksMap
	default:
		return false, fmt.Errorf("unsupported target %q", target)
	}

	after, _ := json.Marshal(root)
	if existed && bytes.Equal(before, after) {
		return false, nil
	}
	if err := writeJSONAtomic(path, root); err != nil {
		return false, err
	}
	return true, nil
}

func upsertClaudeCodexEvent(existing any, event, cmd string) []any {
	desiredHandler := map[string]any{
		"type":    "command",
		"command": cmd,
		"timeout": DefaultTimeoutSec,
	}
	desiredGroup := map[string]any{
		"hooks": []any{desiredHandler},
	}

	list, _ := existing.([]any)
	out := make([]any, 0, len(list)+1)
	replaced := false
	for _, item := range list {
		group, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		if groupHasWorldHook(group, event) {
			if !replaced {
				out = append(out, desiredGroup)
				replaced = true
			}
			continue
		}
		out = append(out, item)
	}
	if !replaced {
		out = append(out, desiredGroup)
	}
	return out
}

func groupHasWorldHook(group map[string]any, event string) bool {
	rawHandlers, _ := group["hooks"].([]any)
	for _, h := range rawHandlers {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		cmd, _ := hm["command"].(string)
		if isWorldHookCommand(cmd, event) {
			return true
		}
	}
	return false
}

func uninstallFromFile(target, path string) (bool, error) {
	root, existed, err := readJSONMap(path)
	if err != nil {
		return false, err
	}
	if !existed {
		return false, nil
	}
	before, _ := json.Marshal(root)

	switch target {
	case TargetAntigravity:
		delete(root, AntigravityHookName)
	case TargetClaude, TargetCodex:
		hooksMap, _ := root["hooks"].(map[string]any)
		if hooksMap != nil {
			for eventKey, sub := range map[string]string{
				"SessionStart": "session-start",
				"Stop":         "stop",
			} {
				if raw, ok := hooksMap[eventKey]; ok {
					cleaned := removeClaudeCodexEvent(raw, sub)
					if len(cleaned) == 0 {
						delete(hooksMap, eventKey)
					} else {
						hooksMap[eventKey] = cleaned
					}
				}
			}
			if len(hooksMap) == 0 {
				delete(root, "hooks")
			} else {
				root["hooks"] = hooksMap
			}
		}
	default:
		return false, fmt.Errorf("unsupported target %q", target)
	}

	after, _ := json.Marshal(root)
	if bytes.Equal(before, after) {
		return false, nil
	}
	if err := writeJSONAtomic(path, root); err != nil {
		return false, err
	}
	return true, nil
}

func removeClaudeCodexEvent(existing any, event string) []any {
	list, _ := existing.([]any)
	out := make([]any, 0, len(list))
	for _, item := range list {
		group, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		rawHandlers, hasHooks := group["hooks"].([]any)
		if !hasHooks {
			out = append(out, item)
			continue
		}
		kept := make([]any, 0, len(rawHandlers))
		for _, h := range rawHandlers {
			hm, ok := h.(map[string]any)
			if ok {
				if cmd, _ := hm["command"].(string); isWorldHookCommand(cmd, event) {
					continue
				}
			}
			kept = append(kept, h)
		}
		if len(kept) == 0 {
			continue
		}
		cloned := make(map[string]any, len(group))
		for k, v := range group {
			cloned[k] = v
		}
		cloned["hooks"] = kept
		out = append(out, cloned)
	}
	return out
}

func inspectFile(target, path string) (sessionStart bool, stop bool, err error) {
	root, existed, err := readJSONMap(path)
	if err != nil {
		return false, false, err
	}
	if !existed {
		return false, false, nil
	}
	switch target {
	case TargetAntigravity:
		spec, _ := root[AntigravityHookName].(map[string]any)
		if spec == nil {
			return false, false, nil
		}
		if enabled, ok := spec["enabled"].(bool); ok && !enabled {
			return false, false, nil
		}
		return flatHandlersHaveWorld(spec["SessionStart"], "session-start"),
			flatHandlersHaveWorld(spec["Stop"], "stop"),
			nil
	case TargetClaude, TargetCodex:
		hooksMap, _ := root["hooks"].(map[string]any)
		if hooksMap == nil {
			return false, false, nil
		}
		return groupedHandlersHaveWorld(hooksMap["SessionStart"], "session-start"),
			groupedHandlersHaveWorld(hooksMap["Stop"], "stop"),
			nil
	default:
		return false, false, fmt.Errorf("unsupported target %q", target)
	}
}

func flatHandlersHaveWorld(raw any, event string) bool {
	list, _ := raw.([]any)
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if cmd, _ := m["command"].(string); isWorldHookCommand(cmd, event) {
			return true
		}
	}
	return false
}

func groupedHandlersHaveWorld(raw any, event string) bool {
	list, _ := raw.([]any)
	for _, item := range list {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if groupHasWorldHook(group, event) {
			return true
		}
	}
	return false
}

// --- Runtime Hook Handlers ---

func DetectDialect(payload map[string]any, override string) string {
	switch strings.ToLower(strings.TrimSpace(override)) {
	case TargetAntigravity, "agy":
		return DialectAntigravity
	case TargetClaude, TargetCodex, DialectClaudeCodex:
		return DialectClaudeCodex
	}
	if payload == nil {
		return DialectClaudeCodex
	}
	for _, camelKey := range []string{"hookEventName", "conversationId", "continuationActive", "terminationReason", "transcriptPath", "lastUserInput"} {
		if _, ok := payload[camelKey]; ok {
			return DialectAntigravity
		}
	}
	return DialectClaudeCodex
}

func parsePayload(raw []byte) map[string]any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(trimmed, &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

func HandleSessionStart(rawStdin []byte, dialectOverride string, authenticated bool) ([]byte, error) {
	payload := parsePayload(rawStdin)
	dialect := DetectDialect(payload, dialectOverride)
	if !authenticated {
		return []byte("{}\n"), nil
	}

	var resp any
	if dialect == DialectAntigravity {
		resp = map[string]any{
			"injectSteps": []any{
				map[string]any{
					"systemMessage": map[string]any{
						"systemMessage": SessionStartPrompt,
					},
				},
			},
		}
	} else {
		resp = map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":     "SessionStart",
				"additionalContext": SessionStartPrompt,
			},
		}
	}
	return marshalCompactJSON(resp)
}

func HandleStop(rawStdin []byte, dialectOverride string, authenticated bool) ([]byte, error) {
	payload := parsePayload(rawStdin)
	dialect := DetectDialect(payload, dialectOverride)
	if !authenticated {
		return []byte("{}\n"), nil
	}

	if dialect == DialectAntigravity {
		if b, _ := payload["continuationActive"].(bool); b {
			return []byte("{}\n"), nil
		}
		if reason, _ := payload["terminationReason"].(string); reason != "" && reason != "NO_TOOL_CALL" {
			return []byte("{}\n"), nil
		}
	} else {
		if b, _ := payload["stop_hook_active"].(bool); b {
			return []byte("{}\n"), nil
		}
	}

	transcriptPath := firstString(payload, "transcriptPath", "transcript_path")
	lastUser := firstString(payload, "lastUserInput", "prompt")
	lastAssistant := firstString(payload, "last_assistant_message")

	summary := inspectTurn(transcriptPath, lastUser, lastAssistant)
	if summary.AlreadyRemembered || summary.TrivialChitchat {
		return []byte("{}\n"), nil
	}

	var resp any
	if dialect == DialectAntigravity {
		resp = map[string]any{
			"decision": "continue",
			"reason":   StopReviewPrompt,
		}
	} else {
		resp = map[string]any{
			"decision": "block",
			"reason":   StopReviewPrompt,
		}
	}
	return marshalCompactJSON(resp)
}

func marshalCompactJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

type turnSummary struct {
	AlreadyRemembered bool
	TrivialChitchat   bool
}

func inspectTurn(transcriptPath, lastUser, lastAssistant string) turnSummary {
	var (
		hasToolCall       bool
		alreadyRemembered bool
		userText          = strings.TrimSpace(lastUser)
		assistantText     = strings.TrimSpace(lastAssistant)
		parsedTranscript  bool
	)

	if strings.TrimSpace(transcriptPath) != "" {
		if entries, ok := readLastTurnEntries(transcriptPath); ok && len(entries) > 0 {
			parsedTranscript = true
			for _, e := range entries {
				if e.IsUser && e.Text != "" {
					userText = e.Text
				}
				if !e.IsUser && e.Text != "" {
					if assistantText != "" {
						assistantText += "\n" + e.Text
					} else {
						assistantText = e.Text
					}
				}
				if e.HasToolCall {
					hasToolCall = true
				}
				if e.CalledWorldRemember {
					alreadyRemembered = true
				}
			}
		}
	}

	if alreadyRemembered {
		return turnSummary{AlreadyRemembered: true}
	}
	if hasToolCall {
		return turnSummary{}
	}

	// Only skip when we actually observed text and it is clearly trivial chitchat.
	if !parsedTranscript && userText == "" && assistantText == "" {
		return turnSummary{}
	}
	if isTrivialTurnText(userText, assistantText) {
		return turnSummary{TrivialChitchat: true}
	}
	return turnSummary{}
}

type transcriptEntry struct {
	IsUser              bool
	Text                string
	HasToolCall         bool
	CalledWorldRemember bool
}

func readLastTurnEntries(path string) ([]transcriptEntry, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	var all []transcriptEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			continue
		}
		entry, ok := parseTranscriptLine(obj)
		if !ok {
			continue
		}
		all = append(all, entry)
	}
	if len(all) == 0 {
		return nil, false
	}
	lastUserIdx := -1
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].IsUser {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx >= 0 {
		return all[lastUserIdx:], true
	}
	return all, true
}

func parseTranscriptLine(obj map[string]any) (transcriptEntry, bool) {
	// Format 1: Antigravity transcript.jsonl
	// {"type":"USER_INPUT","content":"..."} or {"type":"PLANNER_RESPONSE","content":"...","tool_calls":[...]}
	if typ, _ := obj["type"].(string); typ != "" {
		switch typ {
		case "USER_INPUT":
			text, _ := obj["content"].(string)
			return transcriptEntry{IsUser: true, Text: strings.TrimSpace(text)}, true
		case "PLANNER_RESPONSE":
			text, _ := obj["content"].(string)
			entry := transcriptEntry{Text: strings.TrimSpace(text)}
			if rawCalls, ok := obj["tool_calls"].([]any); ok && len(rawCalls) > 0 {
				entry.HasToolCall = true
				rawJSON, _ := json.Marshal(rawCalls)
				if containsWorldRemember(string(rawJSON)) {
					entry.CalledWorldRemember = true
				}
			}
			return entry, true
		}
	}

	// Format 2: Claude Code / Codex JSONL
	// {"type":"user","message":{"role":"user","content":...}} or {"role":"assistant","content":...}
	role := ""
	if r, _ := obj["role"].(string); r != "" {
		role = strings.ToLower(r)
	} else if t, _ := obj["type"].(string); t == "user" || t == "assistant" {
		role = t
	}
	contentVal := obj["content"]
	if msg, ok := obj["message"].(map[string]any); ok {
		if r, _ := msg["role"].(string); r != "" {
			role = strings.ToLower(r)
		}
		if c, ok := msg["content"]; ok {
			contentVal = c
		}
	}
	if role != "user" && role != "assistant" {
		return transcriptEntry{}, false
	}

	entry := transcriptEntry{IsUser: role == "user"}
	switch c := contentVal.(type) {
	case string:
		entry.Text = strings.TrimSpace(c)
	case []any:
		var parts []string
		for _, block := range c {
			bm, ok := block.(map[string]any)
			if !ok {
				continue
			}
			btype, _ := bm["type"].(string)
			switch btype {
			case "text", "output_text", "input_text":
				if txt, _ := bm["text"].(string); strings.TrimSpace(txt) != "" {
					parts = append(parts, strings.TrimSpace(txt))
				}
			case "tool_use", "function_call":
				entry.HasToolCall = true
				rawJSON, _ := json.Marshal(bm)
				if containsWorldRemember(string(rawJSON)) {
					entry.CalledWorldRemember = true
				}
			case "tool_result":
				// Tool results belong to the ongoing turn, not a fresh human prompt.
				entry.IsUser = false
			}
		}
		entry.Text = strings.Join(parts, "\n")
	}
	return entry, true
}

func containsWorldRemember(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "world remember") || strings.Contains(lower, "world\" remember")
}

var trivialPhrases = map[string]bool{
	"hi": true, "hello": true, "hey": true, "ok": true, "okay": true,
	"thanks": true, "thank you": true, "thx": true, "ty": true, "got it": true,
	"cool": true, "great": true, "nice": true, "sure": true, "yes": true, "no": true,
	"bye": true, "goodbye": true,
	"你好": true, "您好": true, "嗨": true, "在吗": true,
	"好的": true, "好": true, "行": true, "收到": true, "明白": true, "了解": true,
	"谢谢": true, "多谢": true, "感谢": true, "辛苦了": true, "拜拜": true, "再见": true,
}

func isTrivialTurnText(userText, assistantText string) bool {
	normUser := strings.ToLower(strings.Trim(strings.TrimSpace(userText), " \t\n\r.!?,。！？，~～"))
	userRunes := utf8.RuneCountInString(normUser)
	assistantRunes := utf8.RuneCountInString(strings.TrimSpace(assistantText))
	if normUser != "" && trivialPhrases[normUser] && assistantRunes < 120 {
		return true
	}
	return userRunes > 0 && userRunes < 6 && assistantRunes < 25
}
