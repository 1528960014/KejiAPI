package antigravity

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"kejiapi/common"
	"kejiapi/relay/channel"
	"kejiapi/relay/channel/gemini"
	relaycommon "kejiapi/relay/common"
	"kejiapi/relay/constant"
	"kejiapi/relaykit/dto"
	"kejiapi/relaykit/relayconvert"
	"kejiapi/relaykit/types"
	"kejiapi/service"

	"github.com/gin-gonic/gin"
)

// Antigravity (Google Code Assist subscription) speaks a wrapped variant of
// the native Gemini wire format:
//
//	POST {endpoint}/v1internal:generateContent
//	{
//	  "project":   "<gcp-project-id>",
//	  "model":     "<model-name>",
//	  "request":   { ...standard Gemini generateContent body... },
//	  "userAgent": "antigravity",
//	  "requestId": "<random-id>"
//	}
//
// Responses are wrapped in { "response": <standard Gemini response>,
// "traceId": "..." } and must be unwrapped before the standard Gemini
// handlers can process them.
const (
	antigravityUserAgent     = "antigravity"
	antigravityNodeUserAgent = "google-api-nodejs-client/10.3.0"
	antigravityAPIClient     = "gl-node/22.18.0"
	antigravityClientMeta    = `{"ideType":"IDE_UNSPECIFIED","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`
)

// antigravityKey is the JSON credential stored on the channel.
type antigravityKey struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ProjectID    string `json:"project_id"`
	Tier         string `json:"tier"`
	Email        string `json:"email"`
}

func parseAntigravityKey(raw string) (*antigravityKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || !strings.HasPrefix(raw, "{") {
		return nil, errors.New("antigravity channel: key must be a JSON credential object")
	}
	var key antigravityKey
	if err := common.Unmarshal([]byte(raw), &key); err != nil {
		return nil, fmt.Errorf("antigravity channel: invalid credential json: %w", err)
	}
	return &key, nil
}

type Adaptor struct {
}

func (a *Adaptor) wrapGeminiRequest(info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	cred, err := parseAntigravityKey(info.ApiKey)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cred.ProjectID) == "" {
		return nil, errors.New("antigravity channel: project_id is missing from the credential, please re-onboard this subscription channel")
	}
	requestIDBytes := make([]byte, 16)
	if _, err := rand.Read(requestIDBytes); err != nil {
		return nil, err
	}
	return map[string]any{
		"project":   cred.ProjectID,
		"model":     info.UpstreamModelName,
		"request":   request,
		"userAgent": antigravityUserAgent,
		"requestId": hex.EncodeToString(requestIDBytes),
	}, nil
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if err := relayconvert.ApplyGeminiThinkingConfigChecked(request, info); err != nil {
		return nil, err
	}
	if len(request.Contents) > 0 {
		if request.Contents[0].Role == "" {
			request.Contents[0].Role = "user"
		}
		for i := range request.Contents {
			for _, part := range request.Contents[i].Parts {
				if part.FileData != nil {
					if part.FileData.MimeType == "" && strings.Contains(part.FileData.FileUri, "www.youtube.com") {
						part.FileData.MimeType = "video/webm"
					}
				}
			}
		}
	}
	return a.wrapGeminiRequest(info, request)
}

func (a *Adaptor) convertViaGeminiFormat(c *gin.Context, info *relaycommon.RelayInfo, request any) (any, error) {
	result, err := service.ConvertRequest(c, info, types.RelayFormatGemini, request)
	if err != nil {
		return nil, err
	}
	geminiRequest, ok := result.Value.(*dto.GeminiChatRequest)
	if !ok {
		return nil, fmt.Errorf("expected Gemini generateContent request, got %T", result.Value)
	}
	return a.wrapGeminiRequest(info, geminiRequest)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	return a.convertViaGeminiFormat(c, info, req)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("antigravity channel: audio endpoint not supported")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("antigravity channel: image endpoint not supported")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	action := "v1internal:generateContent"
	if info.IsStream {
		action = "v1internal:streamGenerateContent?alt=sse"
		if info.RelayMode == constant.RelayModeGemini {
			info.DisablePing = true
		}
	}
	return fmt.Sprintf("%s/%s", strings.TrimRight(info.ChannelBaseUrl, "/"), action), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	cred, err := parseAntigravityKey(info.ApiKey)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cred.AccessToken) == "" {
		return errors.New("antigravity channel: access_token is required")
	}
	req.Set("Authorization", "Bearer "+cred.AccessToken)
	if req.Get("x-goog-api-key") != "" {
		req.Del("x-goog-api-key")
	}
	// The upstream tier/project detection requires the node client
	// user agent; a plain "antigravity" UA is silently downgraded.
	req.Set("User-Agent", antigravityNodeUserAgent)
	req.Set("X-Goog-Api-Client", antigravityAPIClient)
	req.Set("Client-Metadata", antigravityClientMeta)
	req.Set("Content-Type", "application/json")
	if info.IsStream {
		req.Set("Accept", "text/event-stream")
	} else if req.Get("Accept") == "" {
		req.Set("Accept", "application/json")
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return a.convertViaGeminiFormat(c, info, request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("antigravity channel: embedding endpoint not supported")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return a.convertViaGeminiFormat(c, info, &request)
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// unwrapEnvelope extracts the inner Gemini payload from the Antigravity
// response envelope {"response": {...}, "traceId": "..."}.
func unwrapEnvelope(body []byte) ([]byte, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return body, false
	}
	var envelope struct {
		Response json.RawMessage `json:"response"`
		TraceID  string          `json:"traceId"`
	}
	if err := common.Unmarshal(trimmed, &envelope); err != nil {
		return body, false
	}
	raw := bytes.TrimSpace(envelope.Response)
	if len(raw) == 0 || string(raw) == "null" {
		return body, false
	}
	return raw, true
}

// sseEnvelopeReader unwraps Antigravity SSE frames line by line:
// "data: {\"response\": {...}, ...}" becomes "data: {...}".
type sseEnvelopeReader struct {
	src io.Reader
	in  *bufio.Reader
	buf []byte
	eos bool
}

func newSSEEnvelopeReader(body io.Reader) *sseEnvelopeReader {
	return &sseEnvelopeReader{src: body, in: bufio.NewReaderSize(body, 256*1024)}
}

func (r *sseEnvelopeReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		if r.eos {
			return 0, io.EOF
		}
		line, readErr := r.in.ReadString('\n')
		r.buf = []byte(transformSSELine(line))
		if readErr != nil {
			r.eos = true
		}
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

func (r *sseEnvelopeReader) Close() error {
	if c, ok := r.src.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func transformSSELine(line string) string {
	if !strings.HasPrefix(line, "data:") {
		return line
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "" || payload == "[DONE]" {
		return line
	}
	var envelope struct {
		Response json.RawMessage `json:"response"`
	}
	if err := common.Unmarshal([]byte(payload), &envelope); err != nil {
		return line
	}
	raw := bytes.TrimSpace(envelope.Response)
	if len(raw) == 0 || string(raw) == "null" {
		return line
	}
	return "data: " + string(raw) + "\n"
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		service.CloseResponseBodyGracefully(resp)
		return nil, antigravityUpstreamError(resp.StatusCode, body)
	}

	if info.IsStream {
		resp.Body = newSSEEnvelopeReader(resp.Body)
		resp.ContentLength = -1
	} else {
		body, readErr := io.ReadAll(resp.Body)
		service.CloseResponseBodyGracefully(resp)
		if readErr != nil {
			return nil, types.NewOpenAIError(readErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if unwrapped, ok := unwrapEnvelope(body); ok {
			body = unwrapped
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
	}

	if info.RelayMode == constant.RelayModeResponses {
		if info.IsStream {
			return gemini.GeminiResponsesStreamHandler(c, info, resp)
		}
		return gemini.GeminiResponsesHandler(c, info, resp)
	}

	if info.RelayMode == constant.RelayModeGemini {
		if info.IsStream {
			return gemini.GeminiTextGenerationStreamHandler(c, info, resp)
		}
		return gemini.GeminiTextGenerationHandler(c, info, resp)
	}

	if info.IsStream {
		return gemini.GeminiChatStreamHandler(c, info, resp)
	}
	return gemini.GeminiChatHandler(c, info, resp)
}

// antigravityUpstreamError surfaces the upstream error message from a
// non-2xx response body.
func antigravityUpstreamError(statusCode int, body []byte) *types.NewAPIError {
	msg := strings.TrimSpace(string(body))
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if len(body) > 0 && common.Unmarshal(body, &payload) == nil {
		if payload.Error.Message != "" {
			msg = strings.TrimSpace(payload.Error.Message)
		} else if payload.Message != "" {
			msg = strings.TrimSpace(payload.Message)
		}
	}
	if msg == "" {
		msg = fmt.Sprintf("upstream status %d", statusCode)
	} else if len(msg) > 512 {
		msg = msg[:512]
	}
	return types.NewErrorWithStatusCode(fmt.Errorf("antigravity upstream error: %s", msg), types.ErrorCodeDoRequestFailed, statusCode)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}