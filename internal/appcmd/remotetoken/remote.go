package remotetoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

type remoteSpec struct {
	HTTP *httpRemoteSpec `json:"http"`
}

type httpRemoteSpec struct {
	URL     string       `json:"url"`
	Headers headerPolicy `json:"headers"`
}

type headerPolicy struct {
	Mutations []headerMutation `json:"mutations"`
}

type headerMutation struct {
	Side   string   `json:"side"`
	Action string   `json:"action"`
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

func Generate(directiveSecret, resolverURL, resolverToken string) (string, error) {
	directiveSecret = strings.TrimSpace(directiveSecret)
	resolverToken = strings.TrimSpace(resolverToken)
	if directiveSecret == "" || resolverToken == "" {
		return "", errors.New("directive and resolver secrets are required")
	}
	resolverURL = strings.TrimSpace(resolverURL)
	parsed, err := url.Parse(resolverURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("invalid resolver URL")
	}
	spec := remoteSpec{HTTP: &httpRemoteSpec{URL: parsed.String(), Headers: headerPolicy{Mutations: []headerMutation{{
		Side: "request", Action: "set", Name: "Authorization", Values: []string{"Bearer " + resolverToken},
	}}}}}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(directiveSecret))
	_, _ = mac.Write([]byte(payload))
	return strings.Join([]string{
		"dp", "22", "remote",
		payload, base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),
	}, "."), nil
}
