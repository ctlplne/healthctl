package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// Transcribe uses only an operator's platform API credential. Subscription
// grants authorize a different API surface and must never reach this endpoint.
func (c *Client) Transcribe(ctx context.Context, wav []byte) (string, error) {
	key, err := c.credential(Config{Kind: "openai"})
	if err != nil || key == "" {
		return "", errors.New("Configure the OpenAI API key to enable cloud transcription.")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "recording.wav")
	if err != nil {
		return "", err
	}
	if _, err = file.Write(wav); err != nil {
		return "", err
	}
	for _, field := range [][2]string{{"model", "gpt-4o-transcribe"}, {"language", "en"}, {"response_format", "json"}} {
		if err = writer.WriteField(field[0], field[1]); err != nil {
			return "", err
		}
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIBase+"/audio/transcriptions", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := c.httpClients["official"].Do(req)
	if err != nil {
		return "", errors.New("The transcription service could not be reached. You can retry this recording.")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", responseError(res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	var result struct{ Text string }
	if err != nil || len(b) > 65536 || json.Unmarshal(b, &result) != nil || strings.TrimSpace(result.Text) == "" || len(result.Text) > 8192 {
		return "", errors.New("The service returned no usable transcript.")
	}
	return strings.TrimSpace(result.Text), nil
}
