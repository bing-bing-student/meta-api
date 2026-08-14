package guard

import "encoding/json"

const clientMetaValueMaxLen = 128

type clientMetaPayload struct {
	TZ       string `json:"tz"`
	Lang     string `json:"lang"`
	Langs    string `json:"langs"`
	Screen   string `json:"screen"`
	Viewport string `json:"viewport"`
	PerfNav  string `json:"perfNav"`
}

func applyClientMeta(req *RiskRequest, raw []byte) {
	if req == nil || len(raw) == 0 {
		return
	}

	var meta clientMetaPayload
	if err := json.Unmarshal(raw, &meta); err != nil {
		return
	}

	if req.TZ == "" {
		req.TZ = normalizeClientMetaValue(meta.TZ)
	}
	if req.Lang == "" {
		req.Lang = normalizeClientMetaValue(meta.Lang)
	}
	if req.Langs == "" {
		req.Langs = normalizeClientMetaValue(meta.Langs)
	}
	if req.Screen == "" {
		req.Screen = normalizeClientMetaValue(meta.Screen)
	}
	if req.Viewport == "" {
		req.Viewport = normalizeClientMetaValue(meta.Viewport)
	}
	if req.PerfNav == "" {
		req.PerfNav = normalizeClientMetaValue(meta.PerfNav)
	}
}

func normalizeClientMetaValue(value string) string {
	if len(value) > clientMetaValueMaxLen {
		value = value[:clientMetaValueMaxLen]
	}
	return value
}
