package pay

import (
	"log/slog"

	"modelhub/internal/config"
)

// Config is the validated payment configuration used by the API layer.
type Config struct {
	// CNYPerUSD is 0 when recharge is disabled.
	CNYPerUSD float64
	PublicURL string
}

// Build creates the enabled channels from runtime config. A channel is
// enabled only when all of its required credentials are present; partial
// config is logged and skipped rather than failing startup.
func Build(cfg *config.Config) (Config, map[string]Channel) {
	out := Config{CNYPerUSD: cfg.CNYPerUSD, PublicURL: cfg.PublicURL}
	channels := map[string]Channel{}

	if out.CNYPerUSD > 0 {
		if cfg.YiPayMapiURL != "" && cfg.YiPayPID != "" && cfg.YiPayKey != "" {
			channels["yipay"] = NewYiPay(cfg.YiPayMapiURL, cfg.YiPayPID, cfg.YiPayKey)
		} else {
			slog.Warn("pay: yipay not enabled (need PAY_YIPAY_MAPI_URL, PAY_YIPAY_PID, PAY_YIPAY_KEY)")
		}
		if cfg.AlipayAppID != "" && cfg.AlipayPrivateKey != "" && cfg.AlipayPublicKey != "" {
			if ch, err := NewAlipay(cfg.AlipayAppID, cfg.AlipayPrivateKey, cfg.AlipayPublicKey); err == nil {
				channels["alipay"] = ch
			} else {
				slog.Error("pay: alipay not enabled", "error", err)
			}
		} else {
			slog.Warn("pay: alipay not enabled (need PAY_ALIPAY_APP_ID, PAY_ALIPAY_PRIVATE_KEY, PAY_ALIPAY_PUBLIC_KEY)")
		}
		if cfg.WechatMchID != "" && cfg.WechatAPIV3Key != "" && cfg.WechatMerchantSerial != "" &&
			cfg.WechatPrivateKey != "" && cfg.WechatPlatformKey != "" {
			if ch, err := NewWechat(cfg.WechatMchID, cfg.WechatAppID, cfg.WechatAPIV3Key, cfg.WechatMerchantSerial,
				cfg.WechatPrivateKey, cfg.WechatPlatformKey); err == nil {
				channels["wechat"] = ch
			} else {
				slog.Error("pay: wechat not enabled", "error", err)
			}
		} else {
			slog.Warn("pay: wechat not enabled (need PAY_WECHAT_MCH_ID, PAY_WECHAT_API_V3_KEY, PAY_WECHAT_MERCHANT_SERIAL, PAY_WECHAT_PRIVATE_KEY, PAY_WECHAT_PLATFORM_KEY)")
		}
	} else {
		slog.Warn("pay: online recharge disabled (set PAY_CNY_PER_USD, e.g. 7.2, to enable)")
	}
	return out, channels
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
