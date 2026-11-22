package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallStatusUninstallAcrossTargets(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()

	// Pre-populate existing user configs to ensure non-world settings/hooks are preserved.
	agyPath := filepath.Join(home, ".gemini", "config", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(agyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agyPath, []byte(`{"existing-hook":{"Stop":[{"type":"command","command":"echo hi"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	claudePath := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudePath, []byte(`{"model":"claude-sonnet","hooks":{"Stop":[{"hooks":[{"type":"command","command":"./my-check.sh"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatal(err)
	}

	opts := ManageOptions{
		Target:  TargetAuto,
		Scope:   ScopeUser,
		HomeDir: home,
		WorkDir: work,
	}

	installed, err := Install(opts)
	if err != nil {
		t.Fatalf("Install auto failed: %v", err)
	}
	if len(installed) != 3 {
		t.Fatalf("expected 3 detected targets, got %d: %+v", len(installed), installed)
	}
	for _, st := range installed {
		if !st.Changed || !st.SessionStartInstalled || !st.StopInstalled {
			t.Fatalf("unexpected install status: %+v", st)
		}
	}

	// Second install must be idempotent (Changed == false).
	again, err := Install(opts)
	if err != nil {
		t.Fatalf("second Install failed: %v", err)
	}
	for _, st := range again {
		if st.Changed {
			t.Fatalf("expected idempotent install for %s, got Changed=true", st.Target)
		}
	}

	// Verify existing configs are preserved.
	agyRaw, _ := os.ReadFile(agyPath)
	if !strings.Contains(string(agyRaw), "existing-hook") || !strings.Contains(string(agyRaw), "world hook stop") {
		t.Fatalf("unexpected agy hooks.json: %s", string(agyRaw))
	}
	claudeRaw, _ := os.ReadFile(claudePath)
	if !strings.Contains(string(claudeRaw), `"claude-sonnet"`) || !strings.Contains(string(claudeRaw), "./my-check.sh") || !strings.Contains(string(claudeRaw), "world hook session-start") {
		t.Fatalf("unexpected claude settings.json: %s", string(claudeRaw))
	}

	// Status reports all installed.
	statuses, err := Status(opts)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	for _, st := range statuses {
		if !st.SessionStartInstalled || !st.StopInstalled {
			t.Fatalf("expected installed status for %s: %+v", st.Target, st)
		}
	}

	// Uninstall removes only world hooks while preserving unrelated settings/hooks.
	removed, err := Uninstall(opts)
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	if len(removed) != 3 {
		t.Fatalf("expected 3 uninstalled targets, got %d", len(removed))
	}

	agyRaw, _ = os.ReadFile(agyPath)
	if !strings.Contains(string(agyRaw), "existing-hook") || strings.Contains(string(agyRaw), "world-machine") {
		t.Fatalf("uninstall corrupted agy hooks.json: %s", string(agyRaw))
	}
	claudeRaw, _ = os.ReadFile(claudePath)
	if !strings.Contains(string(claudeRaw), `"claude-sonnet"`) || !strings.Contains(string(claudeRaw), "./my-check.sh") || strings.Contains(string(claudeRaw), "world hook") {
		t.Fatalf("uninstall corrupted claude settings.json: %s", string(claudeRaw))
	}

	statuses, err = Status(opts)
	if err != nil {
		t.Fatalf("Status after uninstall failed: %v", err)
	}
	for _, st := range statuses {
		if st.SessionStartInstalled || st.StopInstalled {
			t.Fatalf("expected uninstalled status for %s: %+v", st.Target, st)
		}
	}
}

func TestHandleSessionStartDialectsAndAuth(t *testing.T) {
	// Unauthenticated silently passes through.
	out, err := HandleSessionStart([]byte(`{"hookEventName":"SessionStart"}`), DialectAuto, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("expected empty JSON when unauthenticated, got %s", string(out))
	}

	// Antigravity dialect.
	out, err = HandleSessionStart([]byte(`{"hookEventName":"SessionStart","conversationId":"c1"}`), DialectAuto, true)
	if err != nil {
		t.Fatal(err)
	}
	var agy map[string]any
	if err := json.Unmarshal(out, &agy); err != nil {
		t.Fatal(err)
	}
	if _, ok := agy["injectSteps"]; !ok {
		t.Fatalf("expected injectSteps for antigravity, got %s", string(out))
	}

	// Claude / Codex dialect.
	out, err = HandleSessionStart([]byte(`{"hook_event_name":"SessionStart","session_id":"s1"}`), DialectAuto, true)
	if err != nil {
		t.Fatal(err)
	}
	var cc map[string]any
	if err := json.Unmarshal(out, &cc); err != nil {
		t.Fatal(err)
	}
	hso, _ := cc["hookSpecificOutput"].(map[string]any)
	if hso == nil || hso["hookEventName"] != "SessionStart" || !strings.Contains(hso["additionalContext"].(string), "world recall") {
		t.Fatalf("unexpected claude/codex SessionStart output: %s", string(out))
	}
}

func TestHandleStopGuardsAndTriggering(t *testing.T) {
	// 1. Antigravity continuationActive prevents loop.
	out, err := HandleStop([]byte(`{"hookEventName":"Stop","continuationActive":true,"terminationReason":"NO_TOOL_CALL"}`), DialectAuto, true)
	if err != nil || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("expected pass-through on continuationActive=true, got %s (err=%v)", string(out), err)
	}

	// 2. Antigravity non-normal termination passes through.
	out, err = HandleStop([]byte(`{"hookEventName":"Stop","continuationActive":false,"terminationReason":"USER_CANCELED"}`), DialectAuto, true)
	if err != nil || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("expected pass-through on USER_CANCELED, got %s (err=%v)", string(out), err)
	}

	// 3. Claude/Codex stop_hook_active prevents loop.
	out, err = HandleStop([]byte(`{"hook_event_name":"Stop","stop_hook_active":true}`), DialectAuto, true)
	if err != nil || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("expected pass-through on stop_hook_active=true, got %s (err=%v)", string(out), err)
	}

	// 4. Trivial chitchat passes through.
	out, err = HandleStop([]byte(`{"hook_event_name":"Stop","stop_hook_active":false,"prompt":"谢谢","last_assistant_message":"不客气！"}`), DialectAuto, true)
	if err != nil || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("expected pass-through on trivial chitchat, got %s (err=%v)", string(out), err)
	}

	// 5. Substantive Antigravity turn triggers decision=continue.
	agyTranscript := filepath.Join(t.TempDir(), "transcript.jsonl")
	_ = os.WriteFile(agyTranscript, []byte(
		`{"type":"USER_INPUT","content":"我们今天敲定了下季度产品发布节奏，改为每月第一个周二发布，由小王负责协调。"}`+"\n"+
			`{"type":"PLANNER_RESPONSE","content":"收到，已经整理好下季度的发布节奏和负责分工方案。"}`+"\n",
	), 0o644)
	payload, _ := json.Marshal(map[string]any{
		"hookEventName":      "Stop",
		"continuationActive": false,
		"terminationReason":  "NO_TOOL_CALL",
		"transcriptPath":     agyTranscript,
	})
	out, err = HandleStop(payload, DialectAuto, true)
	if err != nil {
		t.Fatal(err)
	}
	var agyResp map[string]any
	if err := json.Unmarshal(out, &agyResp); err != nil {
		t.Fatal(err)
	}
	if agyResp["decision"] != "continue" || !strings.Contains(agyResp["reason"].(string), "world remember") {
		t.Fatalf("expected decision=continue for antigravity, got %s", string(out))
	}

	// 6. Transcript that already ran `world remember` in the current turn passes through.
	alreadyTranscript := filepath.Join(t.TempDir(), "already.jsonl")
	_ = os.WriteFile(alreadyTranscript, []byte(
		`{"type":"USER_INPUT","content":"我们今天敲定了下季度产品发布节奏，改为每月第一个周二发布。"}`+"\n"+
			`{"type":"PLANNER_RESPONSE","content":"已记录","tool_calls":[{"name":"run_command","args":{"CommandLine":"world remember \"下季度产品发布节奏改为每月第一个周二\" --source agent --json"}}]}`+"\n",
	), 0o644)
	payload, _ = json.Marshal(map[string]any{
		"hookEventName":      "Stop",
		"continuationActive": false,
		"terminationReason":  "NO_TOOL_CALL",
		"transcriptPath":     alreadyTranscript,
	})
	out, err = HandleStop(payload, DialectAuto, true)
	if err != nil || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("expected pass-through when turn already called world remember, got %s (err=%v)", string(out), err)
	}

	// 7. Substantive Claude/Codex turn triggers decision=block.
	ccTranscript := filepath.Join(t.TempDir(), "cc.jsonl")
	_ = os.WriteFile(ccTranscript, []byte(
		`{"type":"user","message":{"role":"user","content":"帮我总结一下刚才和客户沟通的结论，决定先推进一期试点合作。"}}`+"\n"+
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"好的，客户沟通的核心结论是先推进一期试点合作，范围聚焦核心工作流验证。"}]}}`+"\n",
	), 0o644)
	payload, _ = json.Marshal(map[string]any{
		"hook_event_name":  "Stop",
		"stop_hook_active": false,
		"transcript_path":  ccTranscript,
	})
	out, err = HandleStop(payload, DialectAuto, true)
	if err != nil {
		t.Fatal(err)
	}
	var ccResp map[string]any
	if err := json.Unmarshal(out, &ccResp); err != nil {
		t.Fatal(err)
	}
	if ccResp["decision"] != "block" || !strings.Contains(ccResp["reason"].(string), "world remember") {
		t.Fatalf("expected decision=block for claude/codex, got %s", string(out))
	}

	// 8. Transcript that already received a Stop hook review prompt in the current turn passes through
	// even if continuationActive is false and the turn used other tools without calling world remember.
	reviewedTranscript := filepath.Join(t.TempDir(), "reviewed.jsonl")
	sysLine, _ := json.Marshal(map[string]any{
		"type":    "SYSTEM_MESSAGE",
		"content": "The following is a <SYSTEM_MESSAGE>...\n\n<SYSTEM_MESSAGE>\nStop hook blocked termination: " + StopReviewPrompt + "\n</SYSTEM_MESSAGE>",
	})
	_ = os.WriteFile(reviewedTranscript, []byte(
		`{"type":"USER_INPUT","content":"我好奇为什么这个不需要机柜会被显示为忌口？"}`+"\n"+
			`{"type":"PLANNER_RESPONSE","content":"","tool_calls":[{"name":"read_file","args":{"Path":"/app/memories.tsx"}}]}`+"\n"+
			`{"type":"PLANNER_RESPONSE","content":"因为前端硬编码了 avoid ? 忌口 : 偏好。"}`+"\n"+
			string(sysLine)+"\n"+
			`{"type":"PLANNER_RESPONSE","content":"已定位到标签映射逻辑，如需调整文案随时告诉我。"}`+"\n",
	), 0o644)
	payload, _ = json.Marshal(map[string]any{
		"hookEventName":      "Stop",
		"continuationActive": false,
		"terminationReason":  "NO_TOOL_CALL",
		"transcriptPath":     reviewedTranscript,
	})
	out, err = HandleStop(payload, DialectAuto, true)
	if err != nil || strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("expected pass-through after Stop hook already fired in current turn, got %s (err=%v)", string(out), err)
	}
}

func TestResolveCommandBinAndQuotedPaths(t *testing.T) {
	// Explicit flag wins.
	if got := ResolveCommandBin("wm", func() (string, error) { return "/usr/local/bin/world", nil }, nil); got != "wm" {
		t.Fatalf("expected explicit command, got %q", got)
	}
	// Real world executable path is preserved.
	if got := ResolveCommandBin("", func() (string, error) { return "/Users/dev/.gvm/bin/world", nil }, nil); got != "/Users/dev/.gvm/bin/world" {
		t.Fatalf("expected absolute executable path, got %q", got)
	}
	// Non-world test binary falls back to LookPath, then DefaultCommandBin.
	if got := ResolveCommandBin("", func() (string, error) { return "/tmp/app.test", nil }, func(string) (string, error) { return "/opt/homebrew/bin/world", nil }); got != "/opt/homebrew/bin/world" {
		t.Fatalf("expected lookPath fallback, got %q", got)
	}
	if got := ResolveCommandBin("", func() (string, error) { return "/tmp/app.test", nil }, nil); got != DefaultCommandBin {
		t.Fatalf("expected default fallback, got %q", got)
	}

	// Spaces and .exe paths are quoted and recognized by status/uninstall.
	cmd := commandForEvent(`C:\Program Files\World\world.exe`, "stop")
	if cmd != `"C:\Program Files\World\world.exe" hook stop` {
		t.Fatalf("unexpected quoted command: %q", cmd)
	}
	if !isWorldHookCommand(cmd, "stop") {
		t.Fatalf("expected isWorldHookCommand to recognize %q", cmd)
	}
}
