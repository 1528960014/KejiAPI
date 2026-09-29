package pay

import (
	"context"
	"crypto"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Order is one recharge order to be sent to a payment channel.
type Order struct {
	OutTradeNo   string // our merchant order number
	AmountCNYFen int64  // CNY in fen
	Name         string // product/subject line
	NotifyURL    string // async callback
	ReturnURL    string // sync redirect (optional)
	ClientIP     string
	SubType      string // yipay only: "alipay" | "wxpay"
}

// Payment is what the front end needs to collect money: a QR payload and/or
// a payment page URL.
type Payment struct {
	QRCode string
	PayURL string
}

// NotifyResult is a verified async notification.
type NotifyResult struct {
	OutTradeNo     string
	ChannelTradeNo string
	AmountCNYFen   int64
}

// Channel is one payment backend (yipay / alipay / wechat).
type Channel interface {
	ID() string
	Name() string
	CreateOrder(ctx context.Context, o Order) (*Payment, error)
	// ParseNotify validates the channel signature (and, for wechat,
	// decrypts the resource) and returns the verified payment facts.
	ParseNotify(r *http.Request) (*NotifyResult, error)
}

// CNY formats fen as a "0.00" yuan string.
func CNY(fen int64) string {
	sign := ""
	if fen < 0 {
		sign, fen = "-", -fen
	}
	return fmt.Sprintf("%s%d.%02d", sign, fen/100, fen%100)
}

// ParseCNY converts a "0.00" yuan string (max 2 decimals) to fen.
func ParseCNY(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty amount")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg, s = true, s[1:]
	}
	intPart, decPart, _ := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	if len(decPart) > 2 {
		return 0, fmt.Errorf("amount has more than 2 decimals: %s", s)
	}
	decPart = strings.TrimRight(decPart, "0")
	for len(decPart) < 2 {
		decPart += "0"
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, err
	}
	frac, err := strconv.ParseInt(decPart, 10, 64)
	if err != nil {
		return 0, err
	}
	fen := whole*100 + frac
	if neg {
		fen = -fen
	}
	return fen, nil
}

// CreditMicro converts CNY fen to micro-USD at rate CNY per USD.
func CreditMicro(fen int64, cnyPerUSD float64) int64 {
	if fen <= 0 || cnyPerUSD <= 0 {
		return 0
	}
	return int64(math.Round(float64(fen) / 100.0 / cnyPerUSD * 1_000_000))
}

// --- signature helpers ---

// sortedKVString builds "a=b&c=d" from non-empty params, sorted by key.
// exclude lists keys that never take part (sign, sign_type, ...).
func sortedKVString(params map[string]string, exclude map[string]bool) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if exclude[k] || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	return strings.Join(parts, "&")
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// rsaSign signs with SHA256withRSA and returns base64.
func rsaSign(priv *rsa.PrivateKey, msg string) (string, error) {
	h := sha256.Sum256([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// rsaVerify checks a base64 SHA256withRSA signature.
func rsaVerify(pub *rsa.PublicKey, msg, sigB64 string) error {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return err
	}
	h := sha256.Sum256([]byte(msg))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sig)
}

// --- PEM parsing ---

// parseRSAPrivateKey accepts PKCS8 or PKCS1 PEM (literal "\n" already
// expanded by the config layer).
func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("invalid private key PEM")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if k, ok := key.(*rsa.PrivateKey); ok {
			return k, nil
		}
		return nil, errors.New("private key is not RSA")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("cannot parse RSA private key (expected PKCS8 or PKCS1)")
}

// parseRSAPublicKey accepts PKIX or PKCS1 PEM.
func parseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("invalid public key PEM")
	}
	if key, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if k, ok := key.(*rsa.PublicKey); ok {
			return k, nil
		}
		return nil, errors.New("public key is not RSA")
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("cannot parse RSA public key (expected PKIX or PKCS1)")
}
