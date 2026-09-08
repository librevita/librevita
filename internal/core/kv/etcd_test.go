package kv

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEtcdHTTPGatewayOperations(t *testing.T) {
	var mu sync.RWMutex
	storeData := make(map[string]string) // key -> value

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v3/kv/range":
			var req map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			keyBytes, _ := base64.StdEncoding.DecodeString(req["key"])
			key := string(keyBytes)

			if rangeEndB64, ok := req["range_end"]; ok && rangeEndB64 != "" {
				// ListPrefix
				var kvs []etcdKV
				for k, v := range storeData {
					if strings.HasPrefix(k, key) {
						kvs = append(kvs, etcdKV{
							Key:   base64.StdEncoding.EncodeToString([]byte(k)),
							Value: base64.StdEncoding.EncodeToString([]byte(v)),
						})
					}
				}
				_ = json.NewEncoder(w).Encode(etcdRangeResponse{Kvs: kvs})
				return
			}

			// Single Get
			val, ok := storeData[key]
			if !ok {
				_ = json.NewEncoder(w).Encode(etcdRangeResponse{Kvs: nil})
				return
			}
			kvs := []etcdKV{
				{
					Key:   base64.StdEncoding.EncodeToString([]byte(key)),
					Value: base64.StdEncoding.EncodeToString([]byte(val)),
				},
			}
			_ = json.NewEncoder(w).Encode(etcdRangeResponse{Kvs: kvs})

		case "/v3/kv/put":
			var req map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			keyBytes, _ := base64.StdEncoding.DecodeString(req["key"])
			valBytes, _ := base64.StdEncoding.DecodeString(req["value"])
			storeData[string(keyBytes)] = string(valBytes)
			w.WriteHeader(http.StatusOK)

		case "/v3/kv/deleterange":
			var req map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			keyBytes, _ := base64.StdEncoding.DecodeString(req["key"])
			delete(storeData, string(keyBytes))
			w.WriteHeader(http.StatusOK)

		case "/v3/kv/txn":
			var req etcdTxnRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			// Handle PutIfAbsent compare version == 0
			if len(req.Compare) > 0 && req.Compare[0].Target == "VERSION" {
				keyBytes, _ := base64.StdEncoding.DecodeString(req.Compare[0].Key)
				key := string(keyBytes)
				_, exists := storeData[key]
				if !exists {
					// Absent, so success!
					put := req.Success[0].RequestPut
					valBytes, _ := base64.StdEncoding.DecodeString(put.Value)
					storeData[key] = string(valBytes)
					_ = json.NewEncoder(w).Encode(etcdTxnResponse{Succeeded: true})
					return
				}
				// Already exists, failure
				_ = json.NewEncoder(w).Encode(etcdTxnResponse{Succeeded: false})
				return
			}
			w.WriteHeader(http.StatusBadRequest)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store, err := OpenEtcd(server.URL, "testprefix")
	require.NoError(t, err)
	defer func() { _ = store.Close() }()

	ctx := context.Background()

	// 1. Get absent
	_, err = store.Get(ctx, "k1")
	assert.ErrorIs(t, err, ErrNotFound)

	// 2. Put
	err = store.Put(ctx, "k1", []byte("v1"))
	require.NoError(t, err)

	// 3. Get present
	got, err := store.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("v1"), got)

	// 4. PutIfAbsent (existing -> returns false)
	created, err := store.PutIfAbsent(ctx, "k1", []byte("v1-new"))
	require.NoError(t, err)
	assert.False(t, created)

	// 5. PutIfAbsent (new -> returns true)
	created, err = store.PutIfAbsent(ctx, "k2", []byte("v2"))
	require.NoError(t, err)
	assert.True(t, created)

	// 6. ListPrefix
	entries, err := store.ListPrefix(ctx, "k")
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// 7. GetMany
	results, err := store.GetMany(ctx, []string{"k1", "k2", "missing"})
	require.NoError(t, err)
	assert.Equal(t, []byte("v1"), results["k1"].Value)
	assert.Equal(t, []byte("v2"), results["k2"].Value)
	assert.ErrorIs(t, results["missing"].Err, ErrNotFound)

	// 8. Delete
	err = store.Delete(ctx, "k1")
	require.NoError(t, err)
	_, err = store.Get(ctx, "k1")
	assert.ErrorIs(t, err, ErrNotFound)
}
