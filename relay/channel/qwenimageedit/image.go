package qwenimageedit

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// upstreamImage is the per-image element returned by the FastAPI service.
type upstreamImage struct {
	ImageB64    string `json:"image_b64"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	ContentType string `json:"content_type"`
}

// upstreamResponse mirrors RunResponse from app.py.
type upstreamResponse struct {
	RequestID string                 `json:"request_id"`
	Images    []upstreamImage        `json:"images"`
	Timings   map[string]any         `json:"timings"`
}

func qwenImageEditHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var upstream upstreamResponse
	if err := common.Unmarshal(body, &upstream); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	out := dto.ImageResponse{Created: info.StartTime.Unix()}
	for _, img := range upstream.Images {
		out.Data = append(out.Data, dto.ImageData{B64Json: img.ImageB64})
	}

	jsonResponse, err := common.Marshal(out)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	if _, err := c.Writer.Write(jsonResponse); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	return &dto.Usage{}, nil
}
