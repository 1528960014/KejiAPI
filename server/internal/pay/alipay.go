package pay

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Alipay implements Alipay official 当面付 (face-to-face QR) via
// alipay.trade.precreate with RSA2 signatures. Requires the merchant app to
// have the 当面付 product enabled in the Alipay open platform.
type Alipay struct {
	AppID     string
	appKey    *rsa.PrivateKey
	alipayKey *rsa.PublicKey
	location  *time.Location
}

func NewAlipay(appID, privatePEM, publicPEM string) (*Alipay, error) {
	priv, err := parseRSAPrivateKey(privatePEM)
	if err != nil {
		return nil, fmt.Errorf("alipay: %w", err)
	}
	pub, err := parseRSAPublicKey(publicPEM)
	if err != nil {
		return nil, fmt.Errorf("alipay: %w", err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return &Alipay{AppID: appID, appKey: priv, alipayKey: pub, location: loc}, nil
}

func (a *Alipay) ID() string   { return "alipay" }
func (a *Alipay) Name() string { return "支付宝" }

// signAlipayString builds the RSA2 signature base string: all params except
// sign, non-empty, sorted by key, raw values joined "k=v&" (Alipay signs the
// raw values, no URL encoding).
func signAlipayString(params map[string]string) string {
	return sortedKVString(params, map[string]bool{"sign": true})
}

func (a *Alipay) call(ctx context.Context, bizContent string) (map[string]any, error) {
	params := map[string]string{
		"app_id":      a.AppID,
		"method":      "alipay.trade.precreate",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   time.Now().In(a.location).Format("2006-01-02 15:04:05"),
		"version":     "1.0",
		"biz_content": bizContent,
	}
	sign, err := rsaSign(a.appKey, signAlipayString(params))
	if err != nil {
		return nil, err
	}
	params["sign"] = sign

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openapi.alipay.com/gateway.do", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("alipay: bad response: %w", err)
	}
	return out, nil
}

type alipayPrecreateResp struct {
	Code       string `json:"code"`
	Msg        string `json:"msg"`
	SubCode    string `json:"sub_code"`
	SubMsg     string `json:"sub_msg"`
	OutTradeNo string `json:"out_trade_no"`
	QRCode     string `json:"qr_code"`
}

func (a *Alipay) CreateOrder(ctx context.Context, o Order) (*Payment, error) {
	biz, _ := json.Marshal(map[string]string{
		"out_trade_no": o.OutTradeNo,
		"total_amount": CNY(o.AmountCNYFen),
		"subject":      o.Name,
		"product_code": "FACE_TO_FACE_PAYMENT",
	})
	out, err := a.call(ctx, string(biz))
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(out["alipay_trade_precreate_response"])
	var r alipayPrecreateResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("alipay: bad precreate response: %w", err)
	}
	if r.Code != "10000" {
		detail := r.SubMsg
		if detail == "" {
			detail = r.Msg
		}
		return nil, fmt.Errorf("alipay: precreate failed [%s] %s", r.SubCode, detail)
	}
	if r.QRCode == "" {
		return nil, errors.New("alipay: precreate returned no qr_code")
	}
	return &Payment{QRCode: r.QRCode}, nil
}

func (a *Alipay) ParseNotify(r *http.Request) (*NotifyResult, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("alipay: bad form: %w", err)
	}
	params := map[string]string{}
	for k, v := range r.Form {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	sign := params["sign"]
	if sign == "" {
		return nil, errors.New("alipay: missing sign")
	}
	if err := rsaVerify(a.alipayKey, signAlipayString(params), sign); err != nil {
		return nil, fmt.Errorf("alipay: signature invalid: %w", err)
	}
	if params["app_id"] != a.AppID {
		return nil, errors.New("alipay: app_id mismatch")
	}
	if params["trade_status"] != "TRADE_SUCCESS" && params["trade_status"] != "TRADE_FINISHED" {
		return nil, errors.New("alipay: trade not successful")
	}
	fen, err := ParseCNY(params["total_amount"])
	if err != nil {
		return nil, fmt.Errorf("alipay: bad total_amount: %w", err)
	}
	return &NotifyResult{
		OutTradeNo:     params["out_trade_no"],
		ChannelTradeNo: params["trade_no"],
		AmountCNYFen:   fen,
	}, nil
}
