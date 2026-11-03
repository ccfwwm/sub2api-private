//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyOpenAIImagesAccountParamRulesJSON(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openAIImagesAccountParamRulesKey: map[string]any{
				"drop":   []string{"response_format"},
				"rename": map[string]string{"size": "resolution"},
			},
		},
	}
	body, contentType, err := applyOpenAIImagesAccountParamRules(account, []byte(`{"prompt":"cat","size":"1024x1024","response_format":"b64_json"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "1024x1024", got["resolution"])
	require.NotContains(t, got, "size")
	require.NotContains(t, got, "response_format")
}

func TestApplyOpenAIImagesAccountParamRulesMultipart(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("prompt", "cat"))
	require.NoError(t, writer.WriteField("response_format", "b64_json"))
	part, err := writer.CreatePart(textproto.MIMEHeader{
		"Content-Disposition": {`form-data; name="image"; filename="cat.png"`},
		"Content-Type":        {"image/png"},
	})
	require.NoError(t, err)
	_, err = part.Write([]byte("png-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openAIImagesAccountParamRulesKey: map[string]any{
				"drop":   []string{"response_format"},
				"rename": map[string]string{"prompt": "text"},
			},
		},
	}
	filtered, filteredType, err := applyOpenAIImagesAccountParamRules(account, body.Bytes(), writer.FormDataContentType())
	require.NoError(t, err)
	require.Equal(t, writer.FormDataContentType(), filteredType)

	mediaType, params := parseMultipartContentType(t, filteredType)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(filtered), params["boundary"])
	fields := map[string][]byte{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr != nil {
			break
		}
		data, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		fields[part.FormName()] = data
	}
	require.Equal(t, []byte("cat"), fields["text"])
	require.Equal(t, []byte("png-bytes"), fields["image"])
	require.NotContains(t, fields, "response_format")
}

func TestApplyOpenAIImagesAccountParamRulesConflictAndPassthrough(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			openAIImagesAccountParamRulesKey: map[string]any{
				"rename": map[string]string{"size": "resolution"},
			},
		},
	}
	_, _, err := applyOpenAIImagesAccountParamRules(account, []byte(`{"size":"1K","resolution":"2K"}`), "application/json")
	require.ErrorContains(t, err, "collision")

	plain := []byte(`{"size":"1K"}`)
	unchanged, contentType, err := applyOpenAIImagesAccountParamRules(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, plain, "application/json")
	require.NoError(t, err)
	require.Equal(t, plain, unchanged)
	require.Equal(t, "application/json", contentType)

	oauth, _, err := applyOpenAIImagesAccountParamRules(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: account.Extra}, plain, "application/json")
	require.NoError(t, err)
	require.Equal(t, plain, oauth)
}

func parseMultipartContentType(t *testing.T, contentType string) (string, map[string]string) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	return mediaType, params
}

func TestApplyOpenAIImagesAccountParamRulesSimultaneousRenames(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rename map[string]string
		want   string
	}{
		{"chain", map[string]string{"a": "b", "b": "c"}, `{"b":"first","c":"second"}`},
		{"swap", map[string]string{"a": "b", "b": "a"}, `{"b":"first","a":"second"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			account := newOpenAIImagesAPIKeyAccount()
			account.Extra = map[string]any{openAIImagesAccountParamRulesKey: map[string]any{"rename": tc.rename}}
			got, _, err := applyOpenAIImagesAccountParamRules(account, []byte(`{"a":"first","b":"second"}`), "application/json")
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(got))
			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)
			require.NoError(t, writer.WriteField("a", "first"))
			require.NoError(t, writer.WriteField("b", "second"))
			require.NoError(t, writer.Close())
			got, _, err = applyOpenAIImagesAccountParamRules(account, buf.Bytes(), writer.FormDataContentType())
			require.NoError(t, err)
			form, err := multipart.NewReader(bytes.NewReader(got), writer.Boundary()).ReadForm(1024)
			require.NoError(t, err)
			defer form.RemoveAll()
			var want map[string]string
			require.NoError(t, json.Unmarshal([]byte(tc.want), &want))
			for key, value := range want {
				require.Equal(t, []string{value}, form.Value[key])
			}
		})
	}
}

func TestOpenAIImagesAccountParamRulesRejectInvalidConfig(t *testing.T) {
	for _, rules := range []any{
		map[string]any{"drop": "response_format"},
		map[string]any{"rename": map[string]string{"a": "x", "b": "x"}},
		map[string]any{"rename": map[string]string{"a": "bad field"}},
		map[string]any{"drop": []string{""}},
	} {
		account := newOpenAIImagesAPIKeyAccount()
		account.Extra = map[string]any{openAIImagesAccountParamRulesKey: rules}
		_, err := readOpenAIImagesAccountParamRules(account)
		require.Error(t, err)
	}
}

func TestForwardOpenAIImagesAPIKeyAppliesRulesJSONAndMultipart(t *testing.T) {
	for _, multipartRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "multipart"}[multipartRequest], func(t *testing.T) {
			body := []byte(`{"model":"gpt-image-2","prompt":"draw cat","response_format":"b64_json"}`)
			contentType := "application/json"
			if multipartRequest {
				var buf bytes.Buffer
				writer := multipart.NewWriter(&buf)
				for key, value := range map[string]string{"model": "gpt-image-2", "prompt": "draw cat", "response_format": "b64_json"} {
					require.NoError(t, writer.WriteField(key, value))
				}
				for _, name := range []string{"first.png", "second.png"} {
					part, err := writer.CreateFormFile("image[]", name)
					require.NoError(t, err)
					_, err = part.Write([]byte(name))
					require.NoError(t, err)
				}
				require.NoError(t, writer.Close())
				body, contentType = buf.Bytes(), writer.FormDataContentType()
			}
			c, _ := newOpenAIImagesTestContext(t, body)
			c.Request.Header.Set("Content-Type", contentType)
			if multipartRequest {
				c.Request.URL.Path = "/v1/images/edits"
			}
			recorder := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
			svc := newOpenAIImagesTestService(recorder)
			account := newOpenAIImagesAPIKeyAccount()
			account.Extra = map[string]any{openAIImagesAccountParamRulesKey: map[string]any{
				"drop": []string{"response_format"}, "rename": map[string]string{"prompt": "text", "image[]": "files[]"},
			}}
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			_, err = svc.ForwardImages(context.Background(), c, account, body, parsed, "")
			require.NoError(t, err)
			if !multipartRequest {
				require.Equal(t, contentType, recorder.lastReq.Header.Get("Content-Type"))
				require.JSONEq(t, `{"model":"gpt-image-2","text":"draw cat"}`, string(recorder.lastBody))
				return
			}
			_, params := parseMultipartContentType(t, recorder.lastReq.Header.Get("Content-Type"))
			form, err := multipart.NewReader(bytes.NewReader(recorder.lastBody), params["boundary"]).ReadForm(1024 * 1024)
			require.NoError(t, err)
			defer form.RemoveAll()
			require.Equal(t, []string{"draw cat"}, form.Value["text"])
			require.NotContains(t, form.Value, "response_format")
			require.Len(t, form.File["files[]"], 2)
			for _, file := range form.File["files[]"] {
				f, err := file.Open()
				require.NoError(t, err)
				data, err := io.ReadAll(f)
				_ = f.Close()
				require.NoError(t, err)
				require.Equal(t, file.Filename, string(data))
			}
		})
	}
}
