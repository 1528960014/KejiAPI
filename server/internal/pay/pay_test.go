package pay

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestCNYRoundTrip(t *testing.T) {
	cases := []int64{0, 1, 10, 99, 100, 1001, 100000000}
	for _, fen := range cases {
		s := CNY(fen)
		back, err := ParseCNY(s)
		if err != nil {
			t.Fatalf("ParseCNY(%q): %v", s, err)
		}
		if back != fen {
			t.Fatalf("round trip %d -> %q -> %d", fen, s, back)
		}
	}
}

func TestParseCNYErrors(t *testing.T) {
	if _, err := ParseCNY(""); err == nil {
		t.Fatal("empty should fail")
	}
	if _, err := ParseCNY("1.999"); err == nil {
		t.Fatal("3 decimals should fail")
	}
	if _, err := ParseCNY("abc"); err == nil {
		t.Fatal("non-numeric should fail")
	}
}

func TestCreditMicro(t *testing.T) {
	// ¥100 at 7.2 CNY/USD = 13.888... USD
	if got := CreditMicro(10000, 7.2); got != 13888889 {
		t.Fatalf("CreditMicro(10000, 7.2) = %d, want 13888889", got)
	}
	if got := CreditMicro(0, 7.2); got != 0 {
		t.Fatalf("zero fen should be 0, got %d", got)
	}
	if got := CreditMicro(10000, 0); got != 0 {
		t.Fatalf("zero rate should be 0, got %d", got)
	}
}

func TestYiPaySignatureVector(t *testing.T) {
	// Hand-built canonical string: sorted keys name,pid,type (sign/sign_type
	// excluded), raw values, key appended raw at the end.
	canonical := "name=余额充值&pid=10001&type=alipay" + "mykey123"
	want := md5Hex(canonical)

	params := map[string]string{
		"type":      "alipay",
		"pid":       "10001",
		"name":      "余额充值",
		"sign_type": "MD5",
		"sign":      "stale-should-be-ignored",
	}
	if got := signYiPay(params, "mykey123"); got != want {
		t.Fatalf("sign = %s, want %s", got, want)
	}
}

func TestYiPayNotifyVerify(t *testing.T) {
	y := NewYiPay("http://x", "10001", "mykey123")
	form := url.Values{}
	form.Set("pid", "10001")
	form.Set("trade_no", "TP123")
	form.Set("out_trade_no", "RH123")
	form.Set("type", "alipay")
	form.Set("name", "ModelHub 余额充值")
	form.Set("money", "10.00")
	form.Set("trade_status", "TRADE_SUCCESS")
	form.Set("sign_type", "MD5")
	m := map[string]string{}
	for k, v := range form {
		m[k] = v[0]
	}
	form.Set("sign", signYiPay(m, "mykey123"))

	req := httptest.NewRequest(http.MethodPost, "/pay/notify/yipay", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := y.ParseNotify(req)
	if err != nil {
		t.Fatalf("ParseNotify: %v", err)
	}
	if res.OutTradeNo != "RH123" || res.ChannelTradeNo != "TP123" || res.AmountCNYFen != 1000 {
		t.Fatalf("unexpected result: %+v", res)
	}

	// Tampered money must fail.
	form.Set("money", "1.00")
	req3 := httptest.NewRequest(http.MethodPost, "/pay/notify/yipay", bytes.NewBufferString(form.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := y.ParseNotify(req3); err == nil {
		t.Fatal("tampered money should fail verification")
	}
}

func genRSAKey(t *testing.T) (*rsa.PrivateKey, string, string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return priv, string(privPEM), string(pubPEM)
}

func TestAlipaySignVerifyRoundTrip(t *testing.T) {
	_, privPEM, pubPEM := genRSAKey(t)
	a, err := NewAlipay("2021000000", privPEM, pubPEM)
	if err != nil {
		t.Fatal(err)
	}
	params := map[string]string{
		"app_id":      "2021000000",
		"method":      "alipay.trade.precreate",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   "2025-07-15 10:00:00",
		"version":     "1.0",
		"biz_content": `{"out_trade_no":"RH1","total_amount":"10.00"}`,
	}
	sig, err := rsaSign(a.appKey, signAlipayString(params))
	if err != nil {
		t.Fatal(err)
	}
	if err := rsaVerify(a.alipayKey, signAlipayString(params), sig); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	params["total_amount"] = "1.00"
	if err := rsaVerify(a.alipayKey, signAlipayString(params), sig); err == nil {
		t.Fatal("tampered params should fail verification")
	}
}

func TestAlipayNotifyVerify(t *testing.T) {
	_, privPEM, pubPEM := genRSAKey(t)
	a, err := NewAlipay("2021000000", privPEM, pubPEM)
	if err != nil {
		t.Fatal(err)
	}
	params := map[string]string{
		"app_id":       "2021000000",
		"out_trade_no": "RH1",
		"trade_no":     "2025071522001",
		"total_amount": "10.00",
		"trade_status": "TRADE_SUCCESS",
		"sign_type":    "RSA2",
		"notify_time":  "2025-07-15 10:05:00",
	}
	sig, err := rsaSign(a.appKey, signAlipayString(params))
	if err != nil {
		t.Fatal(err)
	}
	params["sign"] = sig
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req := httptest.NewRequest(http.MethodPost, "/pay/notify/alipay", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := a.ParseNotify(req)
	if err != nil {
		t.Fatalf("ParseNotify: %v", err)
	}
	if res.OutTradeNo != "RH1" || res.ChannelTradeNo != "2025071522001" || res.AmountCNYFen != 1000 {
		t.Fatalf("unexpected: %+v", res)
	}
}

// TestWechatParseNotify simulates a full v3 notify: platform-signed header +
// AES-256-GCM encrypted resource.
func TestWechatParseNotify(t *testing.T) {
	priv, privPEM, pubPEM := genRSAKey(t)
	apiKey := "0123456789abcdef0123456789abcdef" // 32 chars
	w, err := NewWechat("1900000001", "", apiKey, "SERIAL1", privPEM, pubPEM)
	if err != nil {
		t.Fatal(err)
	}

	plain, _ := json.Marshal(map[string]any{
		"out_trade_no":   "RH1",
		"transaction_id": "4200001234",
		"trade_state":    "SUCCESS",
		"amount":         map[string]int64{"total": 1000},
	})
	block, err := aes.NewCipher([]byte(apiKey))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := []byte("0123456789ab")
	ciphertext := gcm.Seal(nil, nonce, plain, []byte("transaction"))
	envelope := map[string]any{
		"id":          "EV1",
		"create_time": "2025-07-15T10:00:00+08:00",
		"event_type":  "TRANSACTION.SUCCESS",
		"resource": map[string]string{
			"original_type":   "transaction",
			"algorithm":       "AEAD_AES_256_GCM",
			"ciphertext":      base64.StdEncoding.EncodeToString(ciphertext),
			"associated_data": "transaction",
			"nonce":           string(nonce),
		},
	}
	body, _ := json.Marshal(envelope)

	ts := "1752560000"
	nonceStr := "abc123"
	serial := "SERIAL1"
	verbatim := fmt.Sprintf("%s\n%s\n%s\n\n%s\n", ts, nonceStr, serial, string(body))
	h := sha256.Sum256([]byte(verbatim))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, h[:])
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/pay/notify/wechat", bytes.NewReader(body))
	req.Header.Set("Wechatpay-Timestamp", ts)
	req.Header.Set("Wechatpay-Nonce", nonceStr)
	req.Header.Set("Wechatpay-Serial", serial)
	req.Header.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("Content-Type", "application/json")

	res, err := w.ParseNotify(req)
	if err != nil {
		t.Fatalf("ParseNotify: %v", err)
	}
	if res.OutTradeNo != "RH1" || res.ChannelTradeNo != "4200001234" || res.AmountCNYFen != 1000 {
		t.Fatalf("unexpected: %+v", res)
	}

	// Wrong APIv3 key must fail decryption.
	w2, err := NewWechat("1900000001", "", "fedcba9876543210fedcba9876543210", "SERIAL1", privPEM, pubPEM)
	if err != nil {
		t.Fatal(err)
	}
	req2 := httptest.NewRequest(http.MethodPost, "/pay/notify/wechat", bytes.NewReader(body))
	req2.Header.Set("Wechatpay-Timestamp", ts)
	req2.Header.Set("Wechatpay-Nonce", nonceStr)
	req2.Header.Set("Wechatpay-Serial", serial)
	req2.Header.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(sig))
	if _, err := w2.ParseNotify(req2); err == nil {
		t.Fatal("wrong api v3 key should fail decryption")
	}

	// Forged signature must fail.
	req3 := httptest.NewRequest(http.MethodPost, "/pay/notify/wechat", bytes.NewReader(body))
	req3.Header.Set("Wechatpay-Timestamp", ts)
	req3.Header.Set("Wechatpay-Nonce", nonceStr)
	req3.Header.Set("Wechatpay-Serial", serial)
	req3.Header.Set("Wechatpay-Signature", base64.StdEncoding.EncodeToString(make([]byte, 256)))
	if _, err := w.ParseNotify(req3); err == nil {
		t.Fatal("forged signature should fail")
	}
}

func TestPEMParsingRejectsGarbage(t *testing.T) {
	if _, err := parseRSAPrivateKey("not a pem"); err == nil {
		t.Fatal("garbage private key should fail")
	}
	if _, err := parseRSAPublicKey("not a pem"); err == nil {
		t.Fatal("garbage public key should fail")
	}
}

func TestMethodsOrder(t *testing.T) {
	ch := map[string]Channel{
		"wechat": &Wechat{},
		"yipay":  &YiPay{},
	}
	got := Methods(ch)
	if len(got) != 2 || got[0] != "yipay" || got[1] != "wechat" {
		t.Fatalf("Methods = %v", got)
	}
}
