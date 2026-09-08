package kv

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"librevita.org/pkg/errors"
)

// EtcdStore is a Store backed by etcd v3 via its native HTTP/JSON gateway under a key prefix.
type EtcdStore struct {
	endpoints []string
	prefix    string
	client    *http.Client
}

// OpenEtcd connects to the given endpoints using the etcd v3 HTTP/JSON gateway.
func OpenEtcd(endpointsStr, prefix string) (*EtcdStore, error) {
	if endpointsStr == "" {
		return nil, errors.New("kv: etcd endpoints are required")
	}
	if prefix == "" {
		return nil, errors.New("kv: etcd prefix is required")
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	rawEndpoints := strings.Split(endpointsStr, ",")
	endpoints := make([]string, 0, len(rawEndpoints))
	for _, ep := range rawEndpoints {
		ep = strings.TrimSpace(ep)
		if ep == "" {
			continue
		}
		if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
			ep = "http://" + ep
		}
		endpoints = append(endpoints, strings.TrimRight(ep, "/"))
	}
	if len(endpoints) == 0 {
		return nil, errors.New("kv: etcd endpoints are required")
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	return &EtcdStore{
		endpoints: endpoints,
		prefix:    prefix,
		client:    client,
	}, nil
}

func (s *EtcdStore) key(logical string) string {
	return s.prefix + logical
}

func (s *EtcdStore) doRequest(ctx context.Context, path string, reqPayload any, respPayload any) error {
	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return errors.Wrap(err, "kv: etcd marshal request")
	}

	var lastErr error
	for _, endpoint := range s.endpoints {
		url := endpoint + path
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return errors.Wrap(err, "kv: etcd new request")
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("kv: etcd http error %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		if respPayload != nil {
			decodeErr := json.NewDecoder(resp.Body).Decode(respPayload)
			_ = resp.Body.Close()
			if decodeErr != nil {
				return errors.Wrap(decodeErr, "kv: etcd decode response")
			}
		} else {
			_ = resp.Body.Close()
		}
		return nil
	}

	if lastErr != nil {
		return errors.Wrap(lastErr, "kv: etcd request failed on all endpoints")
	}
	return errors.New("kv: etcd request failed: no endpoints")
}

type etcdKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type etcdRangeResponse struct {
	Kvs []etcdKV `json:"kvs"`
}

func (s *EtcdStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	fullKey := s.key(key)
	req := map[string]string{
		"key": base64.StdEncoding.EncodeToString([]byte(fullKey)),
	}

	var resp etcdRangeResponse
	if err := s.doRequest(ctx, "/v3/kv/range", req, &resp); err != nil {
		return nil, errors.Wrap(err, "kv: etcd get")
	}

	if len(resp.Kvs) == 0 {
		return nil, ErrNotFound
	}

	val, err := base64.StdEncoding.DecodeString(resp.Kvs[0].Value)
	if err != nil {
		return nil, errors.Wrap(err, "kv: etcd decode value")
	}
	return val, nil
}

func (s *EtcdStore) GetMany(ctx context.Context, keys []string) (map[string]Result, error) {
	return batchGetWithWorkers(ctx, keys, defaultBatchWorkers, s.Get)
}

func (s *EtcdStore) Put(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fullKey := s.key(key)
	req := map[string]string{
		"key":   base64.StdEncoding.EncodeToString([]byte(fullKey)),
		"value": base64.StdEncoding.EncodeToString(value),
	}

	if err := s.doRequest(ctx, "/v3/kv/put", req, nil); err != nil {
		return errors.Wrap(err, "kv: etcd put")
	}
	return nil
}

type etcdCompare struct {
	Result  string `json:"result"`
	Target  string `json:"target"`
	Key     string `json:"key"`
	Version string `json:"version"`
}

type etcdOpPut struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type etcdOpSuccess struct {
	RequestPut etcdOpPut `json:"request_put"`
}

type etcdTxnRequest struct {
	Compare []etcdCompare   `json:"compare"`
	Success []etcdOpSuccess `json:"success"`
}

type etcdTxnResponse struct {
	Succeeded bool `json:"succeeded"`
}

func (s *EtcdStore) PutIfAbsent(ctx context.Context, key string, value []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	fullKey := s.key(key)
	b64Key := base64.StdEncoding.EncodeToString([]byte(fullKey))
	b64Val := base64.StdEncoding.EncodeToString(value)

	req := etcdTxnRequest{
		Compare: []etcdCompare{
			{
				Result:  "EQUAL",
				Target:  "VERSION",
				Key:     b64Key,
				Version: "0",
			},
		},
		Success: []etcdOpSuccess{
			{
				RequestPut: etcdOpPut{
					Key:   b64Key,
					Value: b64Val,
				},
			},
		},
	}

	var resp etcdTxnResponse
	if err := s.doRequest(ctx, "/v3/kv/txn", req, &resp); err != nil {
		return false, errors.Wrap(err, "kv: etcd create")
	}
	return resp.Succeeded, nil
}

func (s *EtcdStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	fullKey := s.key(key)
	req := map[string]string{
		"key": base64.StdEncoding.EncodeToString([]byte(fullKey)),
	}

	if err := s.doRequest(ctx, "/v3/kv/deleterange", req, nil); err != nil {
		return errors.Wrap(err, "kv: etcd delete")
	}
	return nil
}

func (s *EtcdStore) ListPrefix(ctx context.Context, prefix string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	fullPrefix := s.prefix + prefix
	rangeEnd := prefixRangeEnd(fullPrefix)

	req := map[string]string{
		"key":       base64.StdEncoding.EncodeToString([]byte(fullPrefix)),
		"range_end": base64.StdEncoding.EncodeToString([]byte(rangeEnd)),
	}

	var resp etcdRangeResponse
	if err := s.doRequest(ctx, "/v3/kv/range", req, &resp); err != nil {
		return nil, errors.Wrap(err, "kv: etcd list")
	}

	entries := make([]Entry, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		decodedKey, err := base64.StdEncoding.DecodeString(kv.Key)
		if err != nil {
			continue
		}
		logical, ok := strings.CutPrefix(string(decodedKey), s.prefix)
		if !ok {
			continue
		}
		val, err := base64.StdEncoding.DecodeString(kv.Value)
		if err != nil {
			continue
		}
		entries = append(entries, Entry{Key: logical, Value: val})
	}
	return entries, nil
}

func prefixRangeEnd(prefix string) string {
	b := []byte(prefix)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < 0xff {
			b[i]++
			return string(b[:i+1])
		}
	}
	return "\x00"
}

func (s *EtcdStore) Close() error {
	s.client.CloseIdleConnections()
	return nil
}
