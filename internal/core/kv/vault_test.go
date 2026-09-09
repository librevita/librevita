package kv

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenVault_Validation(t *testing.T) {
	_, err := OpenVault("", "token", "secret", "prefix/")
	assert.Error(t, err)

	_, err = OpenVault("http://localhost:8200", "", "secret", "prefix/")
	assert.Error(t, err)

	store, err := OpenVault("http://localhost:8200", "token", "", "")
	require.NoError(t, err)
	assert.Equal(t, "secret", store.mount)
	assert.Equal(t, "librevita/keystore/", store.prefix)
	assert.NoError(t, store.Close())
}

func TestVaultStore_Operations(t *testing.T) {
	var mu sync.RWMutex
	// path -> base64 value
	vaultData := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		path := r.URL.Path

		// LIST metadata
		if strings.HasPrefix(path, "/v1/secret/metadata/librevita/keystore") && (r.Method == "LIST" || r.URL.Query().Get("list") == "true") {
			var keys []string
			prefix := "librevita/keystore/"
			for p := range vaultData {
				if strings.HasPrefix(p, prefix) {
					k := strings.TrimPrefix(p, prefix)
					keys = append(keys, k)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"keys": keys,
				},
			})
			return
		}

		// DELETE metadata
		if strings.HasPrefix(path, "/v1/secret/metadata/") && r.Method == http.MethodDelete {
			subPath := strings.TrimPrefix(path, "/v1/secret/metadata/")
			if _, ok := vaultData[subPath]; !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"errors": []string{"not found"}})
				return
			}
			delete(vaultData, subPath)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// GET data
		if strings.HasPrefix(path, "/v1/secret/data/") && r.Method == http.MethodGet {
			subPath := strings.TrimPrefix(path, "/v1/secret/data/")
			val, ok := vaultData[subPath]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]any{"errors": []string{"not found"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"data": map[string]any{
						"value": val,
					},
				},
			})
			return
		}

		// PUT data
		if strings.HasPrefix(path, "/v1/secret/data/") && (r.Method == http.MethodPut || r.Method == http.MethodPost) {
			subPath := strings.TrimPrefix(path, "/v1/secret/data/")
			var body struct {
				Data struct {
					Value string `json:"value"`
				} `json:"data"`
				Options struct {
					CAS *int `json:"cas"`
				} `json:"options"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			if body.Options.CAS != nil && *body.Options.CAS == 0 {
				if _, exists := vaultData[subPath]; exists {
					w.WriteHeader(http.StatusBadRequest)
					_ = json.NewEncoder(w).Encode(map[string]any{"errors": []string{"check-and-set parameter did not match the current version"}})
					return
				}
			}

			vaultData[subPath] = body.Data.Value
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"version": 1,
				},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	store, err := OpenVault(server.URL, "test-token", "secret", "librevita/keystore/")
	require.NoError(t, err)
	ctx := context.Background()

	// 1. Get non-existent
	_, err = store.Get(ctx, "key1")
	assert.ErrorIs(t, err, ErrNotFound)

	// 2. Put and Get
	val1 := []byte("secret-payload-1")
	require.NoError(t, store.Put(ctx, "clinic:101", val1))

	got, err := store.Get(ctx, "clinic:101")
	require.NoError(t, err)
	assert.Equal(t, val1, got)

	// 3. PutIfAbsent
	created, err := store.PutIfAbsent(ctx, "clinic:101", []byte("conflict"))
	require.NoError(t, err)
	assert.False(t, created)

	created, err = store.PutIfAbsent(ctx, "clinic:102", []byte("val102"))
	require.NoError(t, err)
	assert.True(t, created)

	// 4. GetMany
	res, err := store.GetMany(ctx, []string{"clinic:101", "clinic:102", "clinic:999"})
	require.NoError(t, err)
	assert.Equal(t, val1, res["clinic:101"].Value)
	assert.Equal(t, []byte("val102"), res["clinic:102"].Value)
	assert.ErrorIs(t, res["clinic:999"].Err, ErrNotFound)

	// 5. ListPrefix
	entries, err := store.ListPrefix(ctx, "clinic:")
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// 6. Delete and Shred
	require.NoError(t, store.Delete(ctx, "clinic:101"))
	_, err = store.Get(ctx, "clinic:101")
	assert.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, store.Shred(ctx, "clinic:102"))
	_, err = store.Get(ctx, "clinic:102")
	assert.ErrorIs(t, err, ErrNotFound)

	// Deleting non-existent should succeed idempotently
	require.NoError(t, store.Delete(ctx, "clinic:ghost"))
}

func TestVaultListKeys_Nil(t *testing.T) {
	assert.Nil(t, vaultListKeys(nil))
}
