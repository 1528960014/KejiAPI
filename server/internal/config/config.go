package config

import (
	"errors"
	"os"
	"strconv"
)

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Port        string
	DatabaseURL string
	RedisURL    string
	MasterKey   string
	LogLevel    string
	// MediaDir is where composed drama videos are written and served from
	// (GET /media/dramas/{uuid}.mp4).
	MediaDir string

	// P2-4: online recharge (CNY payment -> USD balance).
	// CNYPerUSD is the operator-set rate (e.g. 7.2 = 1 USD costs 7.2 CNY);
	// 0 means recharge is disabled even if channel credentials exist.
	// PublicURL is the externally reachable base URL used for payment
	// notify/return URLs (falls back to the request host when empty).
	CNYPerUSD float64
	PublicURL string

	// YiPay (open-source 易支付 mapi).
	YiPayMapiURL string
	YiPayPID     string
	YiPayKey     string

	// Alipay official (当面付 precreate, RSA2).
	AlipayAppID      string
	AlipayPrivateKey string
	AlipayPublicKey  string

	// WeChat Pay official (v3 native, QR code).
	WechatMchID          string
	WechatAppID          string // optional for native
	WechatAPIV3Key       string
	WechatMerchantSerial string
	WechatPrivateKey     string
	WechatPlatformKey    string // WeChat Pay platform public key (PEM)
}

// Load reads configuration from the environment and validates required values.
func Load() (*Config, error) {
	cfg := &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		RedisURL:    os.Getenv("REDIS_URL"),
		MasterKey:   os.Getenv("MASTER_KEY"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		MediaDir:    getEnv("MODELHUB_MEDIA_DIR", "./media"),

		PublicURL: os.Getenv("PAY_PUBLIC_URL"),

		YiPayMapiURL: os.Getenv("PAY_YIPAY_MAPI_URL"),
		YiPayPID:     os.Getenv("PAY_YIPAY_PID"),
		YiPayKey:     os.Getenv("PAY_YIPAY_KEY"),

		AlipayAppID:      os.Getenv("PAY_ALIPAY_APP_ID"),
		AlipayPrivateKey: expandNL(os.Getenv("PAY_ALIPAY_PRIVATE_KEY")),
		AlipayPublicKey:  expandNL(os.Getenv("PAY_ALIPAY_PUBLIC_KEY")),

		WechatMchID:          os.Getenv("PAY_WECHAT_MCH_ID"),
		WechatAppID:          os.Getenv("PAY_WECHAT_APP_ID"),
		WechatAPIV3Key:       os.Getenv("PAY_WECHAT_API_V3_KEY"),
		WechatMerchantSerial: os.Getenv("PAY_WECHAT_MERCHANT_SERIAL"),
		WechatPrivateKey:     expandNL(os.Getenv("PAY_WECHAT_PRIVATE_KEY")),
		WechatPlatformKey:    expandNL(os.Getenv("PAY_WECHAT_PLATFORM_KEY")),
	}
	if v := os.Getenv("PAY_CNY_PER_USD"); v != "" {
		rate, err := strconv.ParseFloat(v, 64)
		if err != nil || rate <= 0 || rate > 100 {
			return nil, errors.New("PAY_CNY_PER_USD must be a number between 0 and 100 (e.g. 7.2)")
		}
		cfg.CNYPerUSD = rate
	}
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if cfg.MasterKey == "" {
		return nil, errors.New("MASTER_KEY is required")
	}
	if len(cfg.MasterKey) < 16 {
		return nil, errors.New("MASTER_KEY must be at least 16 characters")
	}
	return cfg, nil
}

// expandNL turns literal "\n" sequences in env values into real newlines so
// PEM keys can live in a single .env line.
func expandNL(s string) string {
	out := s
	for i := 0; i < len(out); i++ {
		if out[i] == '\\' && i+1 < len(out) && out[i+1] == 'n' {
			out = out[:i] + "\n" + out[i+2:]
			i++
		}
	}
	return out
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
