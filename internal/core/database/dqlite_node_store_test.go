package database

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/canonical/go-dqlite/v3/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSRVResolver returns canned SRV records; empty/error reproduce a
// record with no targets and a resolution failure.
type fakeSRVResolver struct {
	records []*net.SRV
	err     error
	calls   int
}

func (f *fakeSRVResolver) LookupSRV(_ context.Context, _, _, _ string) (string, []*net.SRV, error) {
	f.calls++
	if f.err != nil {
		return "", nil, f.err
	}
	return "", f.records, nil
}

func nodeAddrs(infos []client.NodeInfo) []string {
	out := make([]string, 0, len(infos))
	for _, n := range infos {
		out = append(out, n.Address)
	}
	return out
}

func TestNodeStoreSRV(t *testing.T) {
	srv := &fakeSRVResolver{records: []*net.SRV{
		{Target: "node3.svc.cluster.local.", Port: 9001},
		{Target: "node1.svc.cluster.local.", Port: 9001},
		{Target: "node1.svc.cluster.local.", Port: 9001}, // duplicate
	}}

	ns := newNodeStore("static1:9001", "_dqlite._tcp.librevita.svc.cluster.local", srv)
	infos, err := ns.Get(context.Background())
	require.NoError(t, err)

	want := []string{"node1.svc.cluster.local:9001", "node3.svc.cluster.local:9001"}
	assert.Equal(t, want, nodeAddrs(infos))
	assert.Equal(t, 1, srv.calls)
}

func TestNodeStoreSRVEmptyFallsBackToStatic(t *testing.T) {
	ns := newNodeStore("node1:9001, node2:9001,node1:9001", "_dqlite._tcp.librevita.svc.cluster.local", &fakeSRVResolver{})
	infos, err := ns.Get(context.Background())
	require.NoError(t, err)
	want := []string{"node1:9001", "node2:9001"}
	assert.Equal(t, want, nodeAddrs(infos))
}

func TestNodeStoreSRVErrorFallsBackToStatic(t *testing.T) {
	ns := newNodeStore("node1:9001", "_dqlite._tcp.librevita.svc.cluster.local", &fakeSRVResolver{err: errors.New("dns down")})
	infos, err := ns.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"node1:9001"}, nodeAddrs(infos))
}

func TestNodeStoreStaticOnly(t *testing.T) {
	ns := newNodeStore("node1:9001,node2:9001", "", nil)
	infos, err := ns.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"node1:9001", "node2:9001"}, nodeAddrs(infos))
}

func TestNodeStoreNoCandidates(t *testing.T) {
	ns := newNodeStore("", "", nil)
	infos, err := ns.Get(context.Background())
	require.NoError(t, err)
	assert.Empty(t, infos)
}

func TestSplitSRV(t *testing.T) {
	for _, tc := range []struct {
		in                   string
		service, proto, name string
		ok                   bool
	}{
		{"_dqlite._tcp.librevita.svc.cluster.local", "dqlite", "tcp", "librevita.svc.cluster.local", true},
		{"dqlite.tcp.librevita", "dqlite", "tcp", "librevita", true},
		{"_dqlite._tcp", "", "", "", false},
		{"", "", "", "", false},
		{"librevita", "", "", "", false},
	} {
		service, proto, name, ok := splitSRV(tc.in)
		assert.Equal(t, tc.service, service, "splitSRV(%q) service", tc.in)
		assert.Equal(t, tc.proto, proto, "splitSRV(%q) proto", tc.in)
		assert.Equal(t, tc.name, name, "splitSRV(%q) name", tc.in)
		assert.Equal(t, tc.ok, ok, "splitSRV(%q) ok", tc.in)
	}
}

func TestSplitAddresses(t *testing.T) {
	assert.Equal(t, []string{"node1:9001", "node2:9001"}, splitAddresses(" node1:9001,,node2:9001 , "))
}
