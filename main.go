package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	"encoding/json"
	"strings"
	"unsafe"
)

const abiVersion uint32 = 1

const (
	pluginID      = "deepseek-reasoning-fixer"
	pluginVersion = "0.1.0"

	placeholderReasoningContent = "[reasoning unavailable]"
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type registration struct {
	SchemaVersion uint32       `json:"schema_version"`
	Metadata      metadata     `json:"metadata"`
	Capabilities  capabilities `json:"capabilities"`
}

type metadata struct {
	Name             string `json:"Name"`
	Version          string `json:"Version"`
	Author           string `json:"Author"`
	GitHubRepository string `json:"GitHubRepository"`
	Logo             string `json:"Logo"`
	ConfigFields     []any  `json:"ConfigFields"`
}

type capabilities struct {
	RequestNormalizer bool `json:"request_normalizer"`
}

type requestTransformRequest struct {
	FromFormat string `json:"FromFormat"`
	ToFormat   string `json:"ToFormat"`
	Model      string `json:"Model"`
	Stream     bool   `json:"Stream"`
	Body       []byte `json:"Body"`
}

type payloadResponse struct {
	Body []byte `json:"Body"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var payload []byte
	if request != nil && requestLen > 0 {
		payload = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), payload)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
	_ = len
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func handleMethod(method string, payload []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		return okEnvelopeJSON(registration{
			SchemaVersion: abiVersion,
			Metadata: metadata{
				Name:             pluginID,
				Version:          pluginVersion,
				Author:           "cpa-admin",
				GitHubRepository: "https://github.com/router-for-me/CLIProxyAPI",
				Logo:             "",
				ConfigFields:     []any{},
			},
			Capabilities: capabilities{RequestNormalizer: true},
		})
	case "request.normalize":
		return normalizeRequest(payload)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func normalizeRequest(payload []byte) ([]byte, error) {
	var req requestTransformRequest
	if len(payload) > 0 {
		if errDecode := json.Unmarshal(payload, &req); errDecode != nil {
			return nil, errDecode
		}
	}
	if len(req.Body) == 0 {
		return okEnvelopeJSON(payloadResponse{Body: req.Body})
	}
	model := strings.TrimSpace(req.Model)
	if !strings.Contains(strings.ToLower(model), "deepseek") {
		return okEnvelopeJSON(payloadResponse{Body: req.Body})
	}
	fixed, changed, errFix := fixReasoningContent(req.Body)
	if errFix != nil {
		return nil, errFix
	}
	if !changed {
		return okEnvelopeJSON(payloadResponse{Body: req.Body})
	}
	return okEnvelopeJSON(payloadResponse{Body: fixed})
}

func fixReasoningContent(body []byte) ([]byte, bool, error) {
	var payload map[string]any
	if errUnmarshal := json.Unmarshal(body, &payload); errUnmarshal != nil {
		return nil, false, nil
	}
	effort, ok := payload["reasoning_effort"].(string)
	if !ok || effort == "" || effort == "none" {
		return nil, false, nil
	}
	messagesRaw, ok := payload["messages"].([]any)
	if !ok || len(messagesRaw) == 0 {
		return nil, false, nil
	}
	changed := false
	for _, msgRaw := range messagesRaw {
		msg, ok := msgRaw.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role != "assistant" {
			continue
		}
		existing, hasReasoning := msg["reasoning_content"]
		switch v := existing.(type) {
		case nil:
			// JSON null or missing field: upstream treats as missing, fill below.
		case string:
			if strings.TrimSpace(v) != "" {
				continue
			}
		case []any:
			if len(v) > 0 {
				continue
			}
		default:
			// JSON objects/numbers/bools: keep verbatim, upstream decides.
			continue
		}
		_ = hasReasoning
		msg["reasoning_content"] = placeholderReasoningContent
		changed = true
	}
	if !changed {
		return nil, false, nil
	}
	out, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, false, errMarshal
	}
	return out, true, nil
}

func okEnvelopeJSON(result any) ([]byte, error) {
	raw, errMarshal := json.Marshal(result)
	if errMarshal != nil {
		return nil, errMarshal
	}
	return json.Marshal(envelope{OK: true, Result: json.RawMessage(raw)})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}