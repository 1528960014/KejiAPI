package pay

import (
	"kejiapi/internal/config"
)

// Config is the validated payment configuration used by the API layer.
type Config struct {
	// CNYPerUSD is 0 when recharge is disabled.
	CNYPerUSD float64
	PublicURL string
}

// Methods returns the enabled channel IDs in a stable order.
func Methods(channels map[string]Channel) []string {
	out := []string{}
	for _, id := range []string{"yipay", "alipay", "wechat"} {
		if _, ok := channels[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// EnvDefaults extracts the payment configuration currently present in the
// process environment. The api layer uses it to seed the database on first
// start: env is only the initial default, the admin console
// (/admin/pay-config) is the runtime source of truth.
func EnvDefaults(cfg *config.Config) (cnyPerUSD float64, publicURL string, channels map[string]map[string]string) {
	return cfg.CNYPerUSD, cfg.PublicURL, map[string]map[string]string{
		"yipay":  {"mapi_url": cfg.YiPayMapiURL, "pid": cfg.YiPayPID, "key": cfg.YiPayKey},
		"alipay": {"app_id": cfg.AlipayAppID, "private_key": cfg.AlipayPrivateKey, "public_key": cfg.AlipayPublicKey},
		"wechat": {"mch_id": cfg.WechatMchID, "app_id": cfg.WechatAppID, "api_v3_key": cfg.WechatAPIV3Key,
			"merchant_serial": cfg.WechatMerchantSerial, "private_key": cfg.WechatPrivateKey, "platform_key": cfg.WechatPlatformKey},
	}
}
