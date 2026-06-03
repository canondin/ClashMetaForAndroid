package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dlclark/regexp2"

	"cfa/native/common"

	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/log"
)

var processors = []processor{
	patchExternalController, // must before patchOverride, so we only apply ExternalController in Override settings
	patchOverride,
	patchGeneral,
	patchProfile,
	patchProxyGroups,
	patchDns,
	patchTun,
	patchListeners,
	patchProviders,
	validConfig,
}

type processor func(cfg *config.RawConfig, profileDir string) error

func patchOverride(cfg *config.RawConfig, _ string) error {
	if err := json.NewDecoder(strings.NewReader(ReadOverride(OverrideSlotPersist))).Decode(cfg); err != nil {
		log.Warnln("Apply persist override: %s", err.Error())
	}
	if err := json.NewDecoder(strings.NewReader(ReadOverride(OverrideSlotSession))).Decode(cfg); err != nil {
		log.Warnln("Apply session override: %s", err.Error())
	}

	return nil
}

func patchExternalController(cfg *config.RawConfig, _ string) error {
	cfg.ExternalController = ""
	cfg.ExternalControllerTLS = ""

	return nil
}

func patchGeneral(cfg *config.RawConfig, profileDir string) error {
	cfg.Interface = ""
	cfg.RoutingMark = 0
	if cfg.ExternalController != "" || cfg.ExternalControllerTLS != "" {
		cfg.ExternalUI = profileDir + "/ui"
	}

	return nil
}

func patchProfile(cfg *config.RawConfig, _ string) error {
	cfg.Profile.StoreSelected = false
	cfg.Profile.StoreFakeIP = true

	return nil
}

func patchProxyGroups(cfg *config.RawConfig, _ string) error {
	proxyNames := collectStreamingJPSG(cfg.Proxy)
	jpSorted := sortDirectFirst(proxyNames.jp)
	sgSorted := sortDirectFirst(proxyNames.sg)

	for i := range cfg.ProxyGroup {
		name, _ := cfg.ProxyGroup[i]["name"].(string)
		switch name {
		case "自动选择":
			cfg.ProxyGroup[i]["proxies"] = toInterfaceSlice(append(jpSorted, sgSorted...))
		case "延迟最低":
			cfg.ProxyGroup[i]["proxies"] = toInterfaceSlice(jpSorted)
		}
	}

	return nil
}

func toInterfaceSlice(names []string) []interface{} {
	result := make([]interface{}, len(names))
	for i, n := range names {
		result[i] = n
	}
	return result
}

type streamingProxyNames struct {
	jp []string
	sg []string
}

func collectStreamingJPSG(proxies []map[string]interface{}) streamingProxyNames {
	infoPatterns := []string{"剩余流量", "套餐到期", "只有新加坡", "直连地址", "TG群", "邀请好友"}
	result := streamingProxyNames{}

	for _, p := range proxies {
		name, _ := p["name"].(string)
		if name == "" {
			continue
		}

		if !strings.Contains(name, "流媒体") {
			continue
		}

		skip := false
		for _, pattern := range infoPatterns {
			if strings.Contains(name, pattern) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}

		if strings.Contains(name, "日本") {
			result.jp = append(result.jp, name)
		} else if strings.Contains(name, "新加坡") {
			result.sg = append(result.sg, name)
		}
	}

	return result
}

func sortDirectFirst(names []string) []string {
	direct := make([]string, 0)
	transit := make([]string, 0)
	rest := make([]string, 0)

	for _, n := range names {
		switch {
		case strings.Contains(n, "直连"):
			direct = append(direct, n)
		case strings.Contains(n, "中转"):
			transit = append(transit, n)
		default:
			rest = append(rest, n)
		}
	}

	sort.Strings(direct)
	sort.Strings(transit)
	sort.Strings(rest)

	return append(append(direct, transit...), rest...)
}

func patchDns(cfg *config.RawConfig, _ string) error {
	if !cfg.DNS.Enable {
		cfg.DNS = config.DefaultRawConfig().DNS
		cfg.DNS.Enable = true
		cfg.DNS.NameServer = defaultNameServers
		cfg.DNS.EnhancedMode = C.DNSFakeIP
		cfg.DNS.FakeIPRange = defaultFakeIPRange
		cfg.DNS.FakeIPFilter = defaultFakeIPFilter

		cfg.ClashForAndroid.AppendSystemDNS = true
	}

	if cfg.ClashForAndroid.AppendSystemDNS {
		cfg.DNS.NameServer = append(cfg.DNS.NameServer, "system://")
	}

	return nil
}

func patchTun(cfg *config.RawConfig, _ string) error {
	cfg.Tun.Enable = false
	cfg.Tun.AutoRoute = false
	cfg.Tun.AutoDetectInterface = false
	return nil
}

func patchListeners(cfg *config.RawConfig, _ string) error {
	newListeners := make([]map[string]any, 0, len(cfg.Listeners))
	for _, mapping := range cfg.Listeners {
		if proxyType, existType := mapping["type"].(string); existType {
			switch proxyType {
			case "tproxy", "redir", "tun":
				continue // remove those listeners which is not supported
			}
		}
		newListeners = append(newListeners, mapping)
	}
	cfg.Listeners = newListeners
	return nil
}

func patchProviders(cfg *config.RawConfig, profileDir string) error {
	forEachProviders(cfg, func(index int, total int, key string, provider map[string]any, prefix string) {
		path, _ := provider["path"].(string)
		if len(path) > 0 {
			path = common.ResolveAsRoot(path)
		} else if url, ok := provider["url"].(string); ok {
			path = prefix + "/" + utils.MakeHash([]byte(url)).String() // same as C.GetPathByHash
		} else {
			return // both path and url are empty, maybe inline provider
		}
		provider["path"] = profileDir + "/providers/" + path
	})

	return nil
}

func validConfig(cfg *config.RawConfig, _ string) error {
	if len(cfg.Proxy) == 0 && len(cfg.ProxyProvider) == 0 {
		return errors.New("profile does not contain `proxies` or `proxy-providers`")
	}

	if _, err := regexp2.Compile(cfg.ClashForAndroid.UiSubtitlePattern, 0); err != nil {
		return fmt.Errorf("compile ui-subtitle-pattern: %s", err.Error())
	}

	return nil
}

func process(cfg *config.RawConfig, profileDir string) error {
	for _, p := range processors {
		if err := p(cfg, profileDir); err != nil {
			return err
		}
	}

	return nil
}
