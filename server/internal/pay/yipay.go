package pay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// YiPay implements the open-source 易支付 V1 mapi protocol:
//   - order:  POST {gateway}/mapi.php  (form), sign = md5(sorted "k=v&" + key)
//   - notify: GET/POST to notify_url, same signature rule, answer "success"
type YiPay struct {
	MapiURL string
	PID     string
	Key     string
}

func NewYiPay(mapiURL, pid, key string) *YiPay {
	return &YiPay{MapiURL: strings.TrimRight(mapiURL, "/"), PID: pid, Key: key}
}

func (y *YiPay) ID() string   { return "yipay" }
func (y *YiPay) Name() string { return "易支付" }

// signYiPay builds the 易支付 MD5 signature over non-empty params
// (excluding sign/sign_type), sorted by key, with the merchant key appended
// raw at the end.
func signYiPay(params map[string]string, key string) string {
	exclude := map[string]bool{"sign": true, "sign_type": true}
	return md5Hex(sortedKVString(params, exclude) + key)
}

type yipayMapiResp struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	TradeNo string `json:"trade_no"`
	PayURL  string `json:"payurl"`
	QRCode  string `json:"qrcode"`
}

func (y *YiPay) CreateOrder(ctx context.Context, o Order) (*Payment, error) {
	subType := o.SubType
	if subType != "wxpay" {
		subType = "alipay"
	}
	params := map[string]string{
		"pid":          y.PID,
		"type":         subType,
		"out_trade_no": o.OutTradeNo,
		"name":         o.Name,
		"money":        CNY(o.AmountCNYFen),
		"notify_url":   o.NotifyURL,
		"sign_type":    "MD5",
	}
	if o.ReturnURL != "" {
		params["return_url"] = o.ReturnURL
	}
	if o.ClientIP != "" {
		params["clientip"] = o.ClientIP
	}
	params["sign"] = signYiPay(params, y.Key)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, y.MapiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out yipayMapiResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("yipay: bad response: %w", err)
	}
	if out.Code != 1 {
		return nil, fmt.Errorf("yipay: order failed: %s", out.Msg)
	}
	if out.PayURL == "" && out.QRCode == "" {
		return nil, errors.New("yipay: response has neither payurl nor qrcode")
	}
	return &Payment{PayURL: out.PayURL, QRCode: out.QRCode}, nil
}

func (y *YiPay) ParseNotify(r *http.Request) (*NotifyResult, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("yipay: bad form: %w", err)
	}
	params := map[string]string{}
	for k, v := range r.Form {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	if got, want := params["sign"], signYiPay(params, y.Key); got != want {
		return nil, errors.New("yipay: signature mismatch")
	}
	if params["trade_status"] != "TRADE_SUCCESS" {
		return nil, errors.New("yipay: trade not successful")
	}
	fen, err := ParseCNY(params["money"])
	if err != nil {
		return nil, fmt.Errorf("yipay: bad money: %w", err)
	}
	return &NotifyResult{
		OutTradeNo:     params["out_trade_no"],
		ChannelTradeNo: params["trade_no"],
		AmountCNYFen:   fen,
	}, nil
}
