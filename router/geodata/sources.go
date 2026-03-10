package geodata

// SourcePreset defines a named set of GeoData download URLs.
type SourcePreset struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DirectList  string `json:"direct_list"`
	ProxyList   string `json:"proxy_list"`
	RejectList  string `json:"reject_list"`
	GFWList     string `json:"gfw_list"`
	CNCidr      string `json:"cn_cidr"`
	PrivateCidr string `json:"private_cidr"`
}

// BuiltinPresets returns the list of available GeoData source presets.
func BuiltinPresets() []SourcePreset {
	return []SourcePreset{
		{
			ID:          "loyalsoldier",
			Name:        "Loyalsoldier",
			Description: "Community-maintained China routing rules (recommended)",
			DirectList:  "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/direct-list.txt",
			ProxyList:   "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/proxy-list.txt",
			RejectList:  "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/reject-list.txt",
			GFWList:     "https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/gfw.txt",
			CNCidr:      "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/text/cn.txt",
			PrivateCidr: "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/text/private.txt",
		},
		{
			ID:          "v2fly",
			Name:        "v2fly",
			Description: "Official v2fly community GeoData",
			DirectList:  "https://raw.githubusercontent.com/v2fly/domain-list-community/release/cn.txt",
			ProxyList:   "https://raw.githubusercontent.com/v2fly/domain-list-community/release/geolocation-!cn.txt",
			RejectList:  "https://raw.githubusercontent.com/v2fly/domain-list-community/release/category-ads-all.txt",
			GFWList:     "https://raw.githubusercontent.com/v2fly/domain-list-community/release/gfw.txt",
			CNCidr:      "https://raw.githubusercontent.com/v2fly/geoip/release/text/cn.txt",
			PrivateCidr: "https://raw.githubusercontent.com/v2fly/geoip/release/text/private.txt",
		},
		{
			ID:          "custom",
			Name:        "Custom",
			Description: "User-defined URLs for each data source",
		},
	}
}

// PresetByID returns the preset with the given ID, or nil if not found.
func PresetByID(id string) *SourcePreset {
	presets := BuiltinPresets()
	for i := range presets {
		if presets[i].ID == id {
			return &presets[i]
		}
	}
	return nil
}
