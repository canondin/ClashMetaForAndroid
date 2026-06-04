package config

import (
	"encoding/json"
	"fmt"
	"os"
	P "path"
	"time"

	"github.com/dop251/goja"

	"github.com/metacubex/mihomo/config"
	"github.com/metacubex/mihomo/log"
)

const scriptTimeout = 5 * time.Second

// executeScript runs a user-provided JS script that filters proxies and proxy-groups.
// The script must define a main(config) function that returns the modified config.
// scriptForTest is the script content; if empty, read from profileDir/script.js.
func executeScript(script string, proxies *[]map[string]interface{}, proxyGroups *[]map[string]interface{}) error {
	vm := goja.New()

	// Inject console routing to logcat
	consoleObj := vm.NewObject()
	_ = consoleObj.Set("log", func(msg string) { log.Infoln("[Script] %s", msg) })
	_ = consoleObj.Set("info", func(msg string) { log.Infoln("[Script] %s", msg) })
	_ = consoleObj.Set("warn", func(msg string) { log.Warnln("[Script] %s", msg) })
	_ = consoleObj.Set("error", func(msg string) { log.Errorln("[Script] %s", msg) })
	_ = vm.Set("console", consoleObj)

	// Build config object for JS
	cfgData := map[string]interface{}{
		"proxies":      *proxies,
		"proxy-groups": *proxyGroups,
	}
	configJSON, err := json.Marshal(cfgData)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	_, err = vm.RunString(fmt.Sprintf("var config = JSON.parse(%s);", mustJSONString(configJSON)))
	if err != nil {
		return fmt.Errorf("inject config: %w", err)
	}

	// Run user script (defines main function)
	if _, err = vm.RunString(script); err != nil {
		return fmt.Errorf("script syntax error: %w", err)
	}

	// Execute main(config) and stringify result
	resultVal, err := vm.RunString("JSON.stringify(main(config))")
	if err != nil {
		return fmt.Errorf("script execution error: %w", err)
	}

	// Parse result back
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultVal.String()), &result); err != nil {
		return fmt.Errorf("script returned invalid config: %w", err)
	}

	// Extract proxies
	if p, ok := result["proxies"]; ok {
		pBytes, _ := json.Marshal(p)
		var newProxies []map[string]interface{}
		if err := json.Unmarshal(pBytes, &newProxies); err != nil {
			return fmt.Errorf("invalid proxies: %w", err)
		}
		*proxies = newProxies
	}

	// Extract proxy-groups
	if pg, ok := result["proxy-groups"]; ok {
		pgBytes, _ := json.Marshal(pg)
		var newGroups []map[string]interface{}
		if err := json.Unmarshal(pgBytes, &newGroups); err != nil {
			return fmt.Errorf("invalid proxy-groups: %w", err)
		}
		*proxyGroups = newGroups
	}

	return nil
}

func patchScript(cfg *config.RawConfig, profileDir string) error {
	scriptPath := P.Join(profileDir, "script.js")
	data, err := os.ReadFile(scriptPath)
	if err != nil || len(data) == 0 {
		return nil
	}

	script := string(data)
	log.Infoln("[Script] Executing user script from %s", scriptPath)

	done := make(chan error, 1)
	proxies := &cfg.Proxy
	proxyGroups := &cfg.ProxyGroup

	go func() {
		done <- executeScript(script, proxies, proxyGroups)
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Warnln("[Script] Execution failed: %s, skipping", err.Error())
		} else {
			log.Infoln("[Script] Execution completed successfully")
		}
		return nil
	case <-time.After(scriptTimeout):
		log.Warnln("[Script] Execution timed out, skipping")
		return nil
	}
}

// mustJSONString wraps a JSON byte slice as a JS string literal.
func mustJSONString(data []byte) string {
	escaped, err := json.Marshal(string(data))
	if err != nil {
		panic(err)
	}
	return string(escaped)
}

// ReadScript reads the user script from a profile directory.
func ReadScript(profilePath string) string {
	data, err := os.ReadFile(P.Join(profilePath, "script.js"))
	if err != nil {
		return ""
	}
	return string(data)
}

// WriteScript writes the user script to a profile directory.
func WriteScript(profilePath, content string) {
	_ = os.WriteFile(P.Join(profilePath, "script.js"), []byte(content), 0600)
}
