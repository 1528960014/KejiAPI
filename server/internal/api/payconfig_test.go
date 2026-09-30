package api

import (
	"strings"
	"testing"

	"kejiapi/internal/store"
)

func TestBuildPayState(t *testing.T) {
	// rate 0: everything disabled
	st := buildPayState(store.PaySettings{CNYPerUSD: 0}, nil)
	if len(st.channels) != 0 {
		t.Fatalf("rate 0: want no channels, got %v", st.channels)
	}
	if st.status["yipay"].OK {
		t.Errorf("rate 0: yipay should not be ok")
	}

	stored := map[string]storedChannel{
		"yipay": {
			enabled: true,
			fields:  map[string]string{"mapi_url": "https://pay.example.com/mapi.php", "pid": "1001", "key": "abc"},
		},
		"alipay": {
			enabled: true,
			fields:  map[string]string{"app_id": "2021000000", "private_key": "not-a-pem", "public_key": "not-a-pem"},
		},
		"wechat": {
			enabled: true,
			fields:  map[string]string{"mch_id": "1", "api_v3_key": "k", "merchant_serial": "s", "private_key": "p"},
		},
	}
	st = buildPayState(store.PaySettings{CNYPerUSD: 7.2}, stored)

	if _, ok := st.channels["yipay"]; !ok {
		t.Errorf("yipay: want enabled channel, status=%+v", st.status["yipay"])
	}
	if st.status["yipay"].OK != true {
		t.Errorf("yipay status = %+v, want ok", st.status["yipay"])
	}
	// invalid RSA key: reported, not enabled
	if _, ok := st.channels["alipay"]; ok {
		t.Errorf("alipay: bad key must not produce a channel")
	}
	if st.status["alipay"].OK || st.status["alipay"].Error == "" {
		t.Errorf("alipay status = %+v, want an error", st.status["alipay"])
	}
	// missing platform_key: reported as missing
	if _, ok := st.channels["wechat"]; ok {
		t.Errorf("wechat: incomplete config must not produce a channel")
	}
	if !strings.Contains(st.status["wechat"].Error, "platform_key") {
		t.Errorf("wechat status = %+v, want missing platform_key", st.status["wechat"])
	}

	// disabled channel stays out even with complete credentials
	stored["yipay"] = storedChannel{enabled: false, fields: stored["yipay"].fields}
	st = buildPayState(store.PaySettings{CNYPerUSD: 7.2}, stored)
	if _, ok := st.channels["yipay"]; ok {
		t.Errorf("disabled yipay must not be enabled")
	}
}

func TestMergePayFields(t *testing.T) {
	stored := map[string]string{"mapi_url": "https://old.example.com/mapi.php", "pid": "1001", "key": "secret"}

	// omitted and masked keep stored values
	out, err := mergePayFields("yipay", stored, map[string]any{"key": payMaskedValue})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if out["mapi_url"] != stored["mapi_url"] || out["pid"] != "1001" || out["key"] != "secret" {
		t.Errorf("merge kept = %v, want all stored values", out)
	}

	// non-empty replaces; empty string keeps
	out, err = mergePayFields("yipay", stored, map[string]any{"mapi_url": "https://new.example.com/mapi.php", "pid": ""})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if out["mapi_url"] != "https://new.example.com/mapi.php" || out["pid"] != "1001" {
		t.Errorf("merge replace = %v", out)
	}

	// unknown fields are ignored, only known names are touched
	out, err = mergePayFields("yipay", stored, map[string]any{"bogus": "x"})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if len(out) != 3 {
		t.Errorf("merge unknown = %v, want the 3 known fields only", out)
	}

	// non-string value is rejected
	if _, err := mergePayFields("yipay", stored, map[string]any{"key": 42}); err == nil {
		t.Errorf("non-string field: want error")
	}
}