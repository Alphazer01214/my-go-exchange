package api

import "my-go-exchange/internal/stream"

const (
	wsOpAuth        = "auth"
	wsOpSubscribe   = "subscribe"
	wsOpUnsubscribe = "unsubscribe"
	wsOpPing        = "ping"
)

type wsRequest struct {
	Op             string                 `json:"op"`
	RequestID      string                 `json:"request_id"`
	Token          string                 `json:"token"`
	Streams        []stream.StreamRequest `json:"streams"`
	SubscriptionID string                 `json:"subscription_id"`
}

type wsResponse struct {
	Type      string      `json:"type"`
	RequestID string      `json:"request_id,omitempty"`
	Data      interface{} `json:"data,omitempty"`
	Error     *wsError    `json:"error,omitempty"`
}

type wsError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
