package omnivoice

// OmniVoice (k2-fsa) 是开源零样本多语种 TTS（diffusion language model），
// new-api 通过自建 FastAPI 包装层（OpenAI 兼容协议）访问 .24 上的 8x MTT S4000。
//
// 上游协议：POST {baseURL}/v1/audio/speech，OpenAI 兼容 + 扩展 ref_audio/ref_text/instructions。
// 默认 baseURL: http://127.0.0.1:8211（与 new-api 同机部署）。

var ModelList = []string{
	"omnivoice-v1",
}

var ChannelName = "omnivoice"
