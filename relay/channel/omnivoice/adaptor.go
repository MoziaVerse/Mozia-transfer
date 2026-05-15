package omnivoice

// new-api ↔ OmniVoice 自部署服务（MTT S4000 GPU + FastAPI 包装层）的桥接 adaptor。
//
// 上游讲 OpenAI 兼容协议（POST /v1/audio/speech），所以本 adaptor 的核心动作就是
// 透传 dto.AudioRequest，并把上游返回的二进制 audio body 原样转给客户端。
// 计费按 input 字符数（TTS 行业惯例），usage.TotalTokens = len(text).

import (
	"bytes"
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

type Adaptor struct{}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	base := strings.TrimRight(info.ChannelBaseUrl, "/")
	if base == "" {
		return "", errors.New("omnivoice channel base url is empty")
	}
	if info.RelayMode != relayconstant.RelayModeAudioSpeech {
		return "", fmt.Errorf("omnivoice only supports audio/speech, got relay mode %d", info.RelayMode)
	}
	return base + "/v1/audio/speech", nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	header.Set("Content-Type", "application/json")
	if info.ApiKey != "" {
		header.Set("Authorization", "Bearer "+info.ApiKey)
	}
	return nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	if info.RelayMode != relayconstant.RelayModeAudioSpeech {
		return nil, fmt.Errorf("omnivoice only supports audio/speech, got relay mode %d", info.RelayMode)
	}
	// 上游 FastAPI 直接吃 OpenAI 协议字段 + 我们扩展的 ref_audio/ref_text/instructions，
	// dto.AudioRequest 里这些都有，直接 marshal 透传。
	jsonData, err := common.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("error marshalling omnivoice request: %w", err)
	}
	return bytes.NewReader(jsonData), nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode != relayconstant.RelayModeAudioSpeech {
		return nil, types.NewError(
			fmt.Errorf("omnivoice only supports audio/speech, got relay mode %d", info.RelayMode),
			types.ErrorCodeInvalidRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewErrorWithStatusCode(
			fmt.Errorf("failed to read omnivoice response: %w", readErr),
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusInternalServerError,
		)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/wav"
	}
	// 透传上游性能 metric 头（X-OmniVoice-*），便于客户端观测
	for _, k := range []string{"X-Omnivoice-Generate-Seconds", "X-Omnivoice-Audio-Seconds", "X-Omnivoice-Rtf"} {
		if v := resp.Header.Get(k); v != "" {
			c.Header(k, v)
		}
	}
	c.Data(resp.StatusCode, contentType, body)

	// 按字符数计费：PromptTokens 用 new-api 估算的输入字符数；TotalTokens 同值；
	// 这样 audio_handler.go 里的 PostTextConsumeQuota 会按 model_ratio 算出 ¥ 价格。
	chars := info.GetEstimatePromptTokens()
	return &dto.Usage{
		PromptTokens:     chars,
		CompletionTokens: 0,
		TotalTokens:      chars,
	}, nil
}

// 下面这些方法都是 channel.Adaptor 接口要求，但 OmniVoice 只支持 audio/speech，
// 所以其它入口一律返回 not implemented，避免被误用于 chat/embedding/image 等场景。

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("omnivoice channel does not support chat completions")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("omnivoice channel does not support claude messages")
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("omnivoice channel does not support gemini")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("omnivoice channel does not support image generation")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("omnivoice channel does not support embeddings")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("omnivoice channel does not support rerank")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("omnivoice channel does not support responses API")
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
