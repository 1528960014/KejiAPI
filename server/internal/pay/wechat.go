package pay

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Wechat implements WeChat Pay official (v3) Native payment: the console
// shows a QR code (code_url) and the async notify carries an AES-256-GCM
// encrypted resource verified with the platform public key.
type Wechat struct {
	MchID       string
	AppID       string // optional for native
	APIV3Key    string // 32 chars
	Serial      string // merchant API certificate serial
	mchKey      *rsa.PrivateKey
	platformKey *rsa.PublicKey
}

func NewWechat(mchID, appID, apiV3Key, serial, privatePEM, platformPEM string) (*Wechat, error) {
	if len(apiV3Key) != 32 {
		return nil, errors.New("wechat: API v3 key must be exactly 32 characters")
	}
	priv, err := parseRSAPrivateKey(privatePEM)
	if err != nil {
		return nil, fmt.Errorf("wechat: %w", err)
	}
	pub, err := parseRSAPublicKey(platformPEM)
	if err != nil {
		return nil, fmt.Errorf("wechat: %w", err)
	}
	return &Wechat{MchID: mchID, AppID: appID, APIV3Key: apiV3Key, Serial: serial, mchKey: priv, platformKey: pub}, nil
}

func (w *Wechat) ID() string   { return "wechat" }
func (w *Wechat) Name() string { return "微信支付" }

const wechatNativePath = "/v3/pay/transactions/native"

// signWechat builds the WECHATPAY2-SHA256-RSA2048 authorization value for a
// request verbatim string.
func (w *Wechat) signWechat(method, path string, ts, nonce, body string) (string, error) {
	verbatim := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n", method, path, ts, nonce, body)
	return rsaSign(w.mchKey, verbatim)
}

type wechatNativeResp struct {
	CodeURL string `json:"code_url"`
}

type wechatErrResp struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (w *Wechat) CreateOrder(ctx context.Context, o Order) (*Payment, error) {
	payload := map[string]any{
		"mchid":        w.MchID,
		"description":  o.Name,
		"out_trade_no": o.OutTradeNo,
		"notify_url":   o.NotifyURL,
		"amount":       map[string]any{"total": o.AmountCNYFen, "currency": "CNY"},
	}
	if w.AppID != "" {
		payload["appid"] = w.AppID
	}
	body, _ := json.Marshal(payload)

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig, err := w.signWechat(http.MethodPost, wechatNativePath, ts, hex.EncodeToString(nonce), string(body))
	if err != nil {
		return nil, err
	}
	auth := fmt.Sprintf("WECHATPAY2-SHA256-RSA2048 mchid=%s,noncestr=%s,timestamp=%s,serial=%s,signature=%s",
		w.MchID, hex.EncodeToString(nonce), ts, w.Serial, sig)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mch.weixin.qq.com"+wechatNativePath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var e wechatErrResp
		if json.Unmarshal(raw, &e) == nil && e.Code != "" {
			return nil, fmt.Errorf("wechat: %s %s", e.Code, e.Message)
		}
		return nil, fmt.Errorf("wechat: http %d: %s", resp.StatusCode, string(raw[:min(len(raw), 200)]))
	}
	var out wechatNativeResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("wechat: bad response: %w", err)
	}
	if out.CodeURL == "" {
		return nil, errors.New("wechat: native response has no code_url")
	}
	return &Payment{QRCode: out.CodeURL}, nil
}

// decryptWechatResource AES-256-GCM decrypts the notify resource.
func (w *Wechat) decryptWechatResource(ciphertextB64, nonce, aad string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(w.APIV3Key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, []byte(nonce), ciphertext, []byte(aad))
	if err != nil {
		return "", fmt.Errorf("wechat: resource decrypt failed: %w", err)
	}
	return string(plain), nil
}

type wechatNotifyBody struct {
	TradeState string `json:"trade_state"`
	Amount     struct {
		Total int64 `json:"total"`
	} `json:"amount"`
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
}

type wechatNotifyEnvelope struct {
	Resource struct {
		Algorithm      string `json:"algorithm"`
		Ciphertext     string `json:"ciphertext"`
		AssociatedData string `json:"associated_data"`
		Nonce          string `json:"nonce"`
	} `json:"resource"`
}

func (w *Wechat) ParseNotify(r *http.Request) (*NotifyResult, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	ts := r.Header.Get("Wechatpay-Timestamp")
	nonce := r.Header.Get("Wechatpay-Nonce")
	serial := r.Header.Get("Wechatpay-Serial")
	sign := r.Header.Get("Wechatpay-Signature")
	if ts == "" || nonce == "" || serial == "" || sign == "" {
		return nil, errors.New("wechat: missing signature headers")
	}
	verbatim := ts + "\n" + nonce + "\n" + serial + "\n\n" + string(body) + "\n"
	if err := rsaVerify(w.platformKey, verbatim, sign); err != nil {
		return nil, fmt.Errorf("wechat: signature invalid: %w", err)
	}
	var env wechatNotifyEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("wechat: bad envelope: %w", err)
	}
	plain, err := w.decryptWechatResource(env.Resource.Ciphertext, env.Resource.Nonce, env.Resource.AssociatedData)
	if err != nil {
		return nil, err
	}
	var r2 wechatNotifyBody
	if err := json.Unmarshal([]byte(plain), &r2); err != nil {
		return nil, fmt.Errorf("wechat: bad resource json: %w", err)
	}
	if r2.TradeState != "SUCCESS" {
		return nil, fmt.Errorf("wechat: trade state is %s", r2.TradeState)
	}
	return &NotifyResult{
		OutTradeNo:     r2.OutTradeNo,
		ChannelTradeNo: r2.TransactionID,
		AmountCNYFen:   r2.Amount.Total,
	}, nil
}
