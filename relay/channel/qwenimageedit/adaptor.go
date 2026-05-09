package qwenimageedit

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Adaptor bridges OpenAI-style image-edit requests to the local
// Qwen-Image-Edit-2511 + fal Multiple-Angles LoRA service running on
// MTT S4000 cards (see ~/app/qwen-edit-service).
type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	base := strings.TrimRight(info.ChannelBaseUrl, "/")
	return base + "/run", nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	header.Set("Content-Type", "application/json")
	if info.ApiKey != "" {
		header.Set("Authorization", "Bearer "+info.ApiKey)
	}
	return nil
}

// upstreamPayload mirrors the FastAPI service's RunRequest schema.
type upstreamPayload struct {
	ImageB64           string  `json:"image_b64"`
	Prompt             string  `json:"prompt"`
	NumInferenceSteps  int     `json:"num_inference_steps,omitempty"`
	GuidanceScale      float64 `json:"guidance_scale,omitempty"`
	Seed               *int64  `json:"seed,omitempty"`
}

const sksPrefix = "<sks>"

// stripDataURL drops the "data:image/...;base64," prefix that browsers
// commonly include, leaving only the base64 payload.
func stripDataURL(s string) string {
	if i := strings.Index(s, ";base64,"); i >= 0 {
		return s[i+len(";base64,"):]
	}
	return s
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	payload := upstreamPayload{
		Prompt:            request.Prompt,
		NumInferenceSteps: 8,
		GuidanceScale:     1.0,
	}

	// The fal LoRA was trained with a "<sks> ..." prompt convention. Add the
	// trigger token automatically so callers can use plain-language prompts.
	if !strings.HasPrefix(strings.TrimSpace(payload.Prompt), sksPrefix) {
		payload.Prompt = sksPrefix + " " + payload.Prompt
	}

	// Resolve the input image. ImageRequest.Image is a json.RawMessage that
	// in practice carries a base64-encoded string (optionally with a data:
	// URL prefix) or, less commonly, a URL string.
	if len(request.Image) > 0 {
		var s string
		if err := common.Unmarshal(request.Image, &s); err == nil && s != "" {
			payload.ImageB64 = stripDataURL(s)
		}
	}

	// extra_fields lets callers override num_inference_steps / guidance_scale
	// / seed and supply image_b64 explicitly when needed.
	if len(request.ExtraFields) > 0 {
		if err := common.Unmarshal(request.ExtraFields, &payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal extra_fields: %w", err)
		}
	}

	if payload.ImageB64 == "" {
		return nil, errors.New("qwenimageedit: image is required (provide as base64 in `image` or `extra_fields.image_b64`)")
	}

	return payload, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	url, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	if err := a.SetupRequestHeader(c, &req.Header, info); err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	resp, err := channel.DoRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits {
		return qwenImageEditHandler(c, resp, info)
	}
	return nil, types.NewError(errors.New("qwenimageedit: only image generation/edit relay modes are supported"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

// --- methods required by channel.Adaptor that this image-only channel does
// --- not implement. Returning errors keeps the dispatcher honest if a caller
// --- ever routes a non-image request here.

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("qwenimageedit: chat-completions not supported")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("qwenimageedit: claude messages not supported")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("qwenimageedit: gemini not supported")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("qwenimageedit: rerank not supported")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("qwenimageedit: embeddings not supported")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("qwenimageedit: audio not supported")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("qwenimageedit: openai responses not supported")
}

// Compile-time guard: Adaptor satisfies channel.Adaptor.
var _ channel.Adaptor = (*Adaptor)(nil)
