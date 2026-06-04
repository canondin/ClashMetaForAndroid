package config

import (
	"encoding/json"
	"testing"
)

func makeTestProxies() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "日本流媒体01", "type": "ss", "server": "jp1.example.com"},
		{"name": "日本流媒体02-直连", "type": "vmess", "server": "jp2.example.com"},
		{"name": "新加坡流媒体01", "type": "ss", "server": "sg1.example.com"},
		{"name": "香港01", "type": "trojan", "server": "hk1.example.com"},
		{"name": "剩余流量:100GB", "type": "ss", "server": "info.example.com"},
		{"name": "TG群:JoinUs", "type": "ss", "server": "info2.example.com"},
		{"name": "美国01", "type": "vmess", "server": "us1.example.com"},
	}
}

func makeTestGroups() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":    "自动选择",
			"type":    "select",
			"proxies": []interface{}{"日本流媒体01", "日本流媒体02-直连", "新加坡流媒体01", "DIRECT"},
		},
		{
			"name":    "Proxy",
			"type":    "select",
			"proxies": []interface{}{"自动选择", "DIRECT"},
		},
	}
}

func TestExecuteScript_FilterByKeyword(t *testing.T) {
	script := `
function main(config) {
	config.proxies = config.proxies.filter(p => p.name.includes("日本"));
	return config;
}
`
	proxies := makeTestProxies()
	groups := makeTestGroups()

	err := executeScript(script, &proxies, &groups)
	if err != nil {
		t.Fatalf("executeScript failed: %v", err)
	}

	if len(proxies) != 2 {
		t.Errorf("expected 2 proxies, got %d", len(proxies))
	}
	for _, p := range proxies {
		name := p["name"].(string)
		if !contains(name, "日本") {
			t.Errorf("unexpected proxy: %s", name)
		}
	}
}

func TestExecuteScript_FilterJunkNodes(t *testing.T) {
	script := `
function main(config) {
	var junk = ["剩余流量", "TG群", "套餐到期", "官网"];
	config.proxies = config.proxies.filter(p =>
		!junk.some(k => p.name.includes(k))
	);
	return config;
}
`
	proxies := makeTestProxies()
	groups := makeTestGroups()

	err := executeScript(script, &proxies, &groups)
	if err != nil {
		t.Fatalf("executeScript failed: %v", err)
	}

	if len(proxies) != 5 {
		t.Errorf("expected 5 proxies (junk removed), got %d", len(proxies))
	}
	for _, p := range proxies {
		name := p["name"].(string)
		if contains(name, "剩余流量") || contains(name, "TG群") {
			t.Errorf("junk node not filtered: %s", name)
		}
	}
}

func TestExecuteScript_RebuildProxyGroups(t *testing.T) {
	script := `
function main(config) {
	var all = config.proxies.map(p => p.name);
	var jp = all.filter(n => n.includes("日本"));
	var sg = all.filter(n => n.includes("新加坡"));

	config["proxy-groups"] = [
		{ name: "Proxy", type: "select", proxies: ["Auto", "JP", "SG", "DIRECT"] },
		{ name: "Auto", type: "url-test", proxies: all,
		  url: "http://www.gstatic.com/generate_204", interval: 300 },
		{ name: "JP", type: "url-test", proxies: jp,
		  url: "http://www.gstatic.com/generate_204", interval: 300 },
		{ name: "SG", type: "url-test", proxies: sg,
		  url: "http://www.gstatic.com/generate_204", interval: 300 },
	];
	return config;
}
`
	proxies := makeTestProxies()
	groups := makeTestGroups()

	err := executeScript(script, &proxies, &groups)
	if err != nil {
		t.Fatalf("executeScript failed: %v", err)
	}

	if len(groups) != 4 {
		t.Fatalf("expected 4 proxy-groups, got %d", len(groups))
	}

	// Check JP group
	jpGroup := groups[2]
	if jpGroup["name"].(string) != "JP" {
		t.Errorf("expected group name JP, got %v", jpGroup["name"])
	}
	jpProxies := toStringSlice(jpGroup["proxies"])
	if len(jpProxies) != 2 {
		t.Errorf("expected 2 JP proxies, got %d", len(jpProxies))
	}
}

func TestExecuteScript_EmptyScript(t *testing.T) {
	proxies := makeTestProxies()
	groups := makeTestGroups()

	// Empty script should still work (main returns config unchanged)
	script := `function main(config) { return config; }`

	err := executeScript(script, &proxies, &groups)
	if err != nil {
		t.Fatalf("executeScript failed: %v", err)
	}

	if len(proxies) != 7 {
		t.Errorf("expected 7 proxies (unchanged), got %d", len(proxies))
	}
}

func TestExecuteScript_SyntaxError(t *testing.T) {
	script := `function main(config) { invalid syntax !!! }`

	proxies := makeTestProxies()
	groups := makeTestGroups()

	err := executeScript(script, &proxies, &groups)
	if err == nil {
		t.Fatal("expected error for syntax error, got nil")
	}
}

func TestExecuteScript_RuntimeError(t *testing.T) {
	script := `function main(config) { throw new Error("boom"); }`

	proxies := makeTestProxies()
	groups := makeTestGroups()

	err := executeScript(script, &proxies, &groups)
	if err == nil {
		t.Fatal("expected error for runtime error, got nil")
	}
}

func TestExecuteScript_MissingMain(t *testing.T) {
	script := `var x = 1;`

	proxies := makeTestProxies()
	groups := makeTestGroups()

	err := executeScript(script, &proxies, &groups)
	if err == nil {
		t.Fatal("expected error when main is not defined, got nil")
	}
}

func TestExecuteScript_ModifyProxyFields(t *testing.T) {
	script := `
function main(config) {
	config.proxies = config.proxies.map(p => {
		p.name = "[Filtered] " + p.name;
		return p;
	});
	return config;
}
`
	proxies := makeTestProxies()
	groups := makeTestGroups()

	err := executeScript(script, &proxies, &groups)
	if err != nil {
		t.Fatalf("executeScript failed: %v", err)
	}

	for _, p := range proxies {
		name := p["name"].(string)
		if !contains(name, "[Filtered] ") {
			t.Errorf("proxy name not modified: %s", name)
		}
	}
}

func TestExecuteScript_SortByLatency(t *testing.T) {
	proxies := []map[string]interface{}{
		{"name": "JP-1", "type": "ss"},
		{"name": "JP-2", "type": "ss"},
		{"name": "JP-3", "type": "ss"},
	}
	script := `
function main(config) {
	config.proxies.reverse();
	return config;
}
`
	groups := makeTestGroups()

	err := executeScript(script, &proxies, &groups)
	if err != nil {
		t.Fatalf("executeScript failed: %v", err)
	}

	if proxies[0]["name"].(string) != "JP-3" {
		t.Errorf("expected JP-3 first after reverse, got %s", proxies[0]["name"])
	}
}

func TestReadScript_NoFile(t *testing.T) {
	dir := t.TempDir()
	content := ReadScript(dir)
	if content != "" {
		t.Errorf("expected empty string for non-existent script, got %q", content)
	}
}

func TestWriteAndReadScript(t *testing.T) {
	dir := t.TempDir()
	expected := "function main(config) { return config; }"

	WriteScript(dir, expected)
	content := ReadScript(dir)

	if content != expected {
		t.Errorf("expected %q, got %q", expected, content)
	}
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toStringSlice(v interface{}) []string {
	b, _ := json.Marshal(v)
	var result []string
	json.Unmarshal(b, &result)
	return result
}
