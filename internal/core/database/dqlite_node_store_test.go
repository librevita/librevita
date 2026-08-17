package database

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/canonical/go-dqlite/v3/client"
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
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"node1.svc.cluster.local:9001", "node3.svc.cluster.local:9001"}
	if got := nodeAddrs(infos); !reflect.DeepEqual(got, want) {
		t.Fatalf("Get = %v, want %v", got, want)
	}
	if srv.calls != 1 {
		t.Fatalf("resolver called %d times, want 1", srv.calls)
	}
}

func TestNodeStoreSRVEmptyFallsBackToStatic(t *testing.T) {
	ns := newNodeStore("node1:9001, node2:9001,node1:9001", "_dqlite._tcp.librevita.svc.cluster.local", &fakeSRVResolver{})
	infos, err := ns.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"node1:9001", "node2:9001"}
	if got := nodeAddrs(infos); !reflect.DeepEqual(got, want) {
		t.Fatalf("Get = %v, want %v", got, want)
	}
}

func TestNodeStoreSRVErrorFallsBackToStatic(t *testing.T) {
	ns := newNodeStore("node1:9001", "_dqlite._tcp.librevita.svc.cluster.local", &fakeSRVResolver{err: errors.New("dns down")})
	infos, err := ns.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := nodeAddrs(infos); !reflect.DeepEqual(got, []string{"node1:9001"}) {
		t.Fatalf("Get = %v, want static fallback", got)
	}
}

func TestNodeStoreStaticOnly(t *testing.T) {
	ns := newNodeStore("node1:9001,node2:9001", "", nil)
	infos, err := ns.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := nodeAddrs(infos); !reflect.DeepEqual(got, []string{"node1:9001", "node2:9001"}) {
		t.Fatalf("Get = %v", got)
	}
}

func TestNodeStoreNoCandidates(t *testing.T) {
	ns := newNodeStore("", "", nil)
	infos, err := ns.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 0 {
		t.Fatalf("Get = %v, want empty", infos)
	}
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
		if service != tc.service || proto != tc.proto || name != tc.name || ok != tc.ok {
			t.Errorf("splitSRV(%q) = %q/%q/%q/%v, want %q/%q/%q/%v",
				tc.in, service, proto, name, ok, tc.service, tc.proto, tc.name, tc.ok)
		}
	}
}

func TestSplitAddresses(t *testing.T) {
	if got := splitAddresses(" node1:9001,,node2:9001 , "); !reflect.DeepEqual(got, []string{"node1:9001", "node2:9001"}) {
		t.Fatalf("splitAddresses = %v", got)
	}
}
