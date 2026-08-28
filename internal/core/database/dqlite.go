package database

import (
	"context"
	"database/sql"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/canonical/go-dqlite/v3/client"
	dqlitedriver "github.com/canonical/go-dqlite/v3/driver"
	"github.com/cockroachdb/errors"
)

// dqliteSrvResolver isolates the DNS lookup (net.Resolver's SRV
// signature) so tests can stub it.
type dqliteSrvResolver interface {
	LookupSRV(ctx context.Context, service, proto, name string) (cname string, addrs []*net.SRV, err error)
}

// nodeStore feeds the dqlite driver its candidate node addresses (the
// driver probes them to find the cluster leader; the cluster itself
// syncs the full membership once connected). It seeds from the static
// addresses and, when a discovery SRV record is configured, re-queries
// it on every Get, so cluster membership changes are picked up without
// restarting the application. The static addresses remain the bootstrap
// fallback when the record is empty or the lookup fails.
type nodeStore struct {
	resolver dqliteSrvResolver
	addrs    []string
	srvSvc   string
	srvProto string
	srvName  string
}

func newNodeStore(addrs string, srv string, resolver dqliteSrvResolver) *nodeStore {
	ns := &nodeStore{
		resolver: resolver,
		addrs:    splitAddresses(addrs),
	}
	if svc, proto, name, ok := splitSRV(srv); ok {
		ns.srvSvc, ns.srvProto, ns.srvName = svc, proto, name
	}
	return ns
}

// Get returns the candidate node addresses. With a discovery SRV the
// record is resolved live; the static list is the fallback, so the
// candidate set is never empty when either source is configured.
func (s *nodeStore) Get(ctx context.Context) ([]client.NodeInfo, error) {
	if s.srvName != "" && s.resolver != nil {
		if _, records, err := s.resolver.LookupSRV(ctx, s.srvSvc, s.srvProto, s.srvName); err == nil && len(records) > 0 {
			return srvNodes(records), nil
		}
	}
	return staticNodes(s.addrs), nil
}

// Set is a no-op: the dqlite driver owns cluster membership once
// connected, and discovery re-runs on every Get.
func (s *nodeStore) Set(context.Context, []client.NodeInfo) error { return nil }

// splitSRV parses an SRV record reference, accepting the conventional
// "_service._proto.name" form as well as a missing leading underscore
// (e.g. "dqlite.tcp.librevita.svc.cluster.local").
func splitSRV(record string) (service, proto, name string, ok bool) {
	parts := strings.Split(record, ".")
	if len(parts) < 3 {
		return "", "", "", false
	}
	service = strings.TrimPrefix(parts[0], "_")
	proto = strings.TrimPrefix(parts[1], "_")
	name = strings.Join(parts[2:], ".")
	if service == "" || proto == "" || name == "" {
		return "", "", "", false
	}
	return service, proto, name, true
}

// srvNodes maps SRV records to deduplicated, stably ordered node
// infos; the record order is DNS round-robin, which would otherwise
// make the candidate list unstable across lookups.
func srvNodes(records []*net.SRV) []client.NodeInfo {
	out := make([]client.NodeInfo, 0, len(records))
	seen := make(map[string]bool)
	for _, r := range records {
		addr := strings.TrimSuffix(r.Target, ".") + ":" + strconv.Itoa(int(r.Port))
		if seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, client.NodeInfo{Address: addr})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out
}

// staticNodes deduplicates the configured node addresses in order.
func staticNodes(addrs []string) []client.NodeInfo {
	if len(addrs) == 0 {
		return nil
	}
	out := make([]client.NodeInfo, 0, len(addrs))
	seen := make(map[string]bool)
	for _, a := range addrs {
		if seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, client.NodeInfo{Address: a})
	}
	return out
}

// splitAddresses trims and filters the comma-separated address list.
func splitAddresses(addrs string) []string {
	var out []string
	for _, addr := range strings.Split(addrs, ",") {
		if addr = strings.TrimSpace(addr); addr != "" {
			out = append(out, addr)
		}
	}
	return out
}

// openDqlite connects to the dqlite cluster through the pure-Go client
// driver (github.com/canonical/go-dqlite/v3). The driver performs real
// transactions (BEGIN/COMMIT via the wire protocol, replicated through
// Raft), prepared statements, and strong consistency. A single
// connection serializes writes, mirroring the SQLite backend.
func openDqlite(addrs string, srv string, database string) (*sql.DB, error) {
	ns := newNodeStore(addrs, srv, net.DefaultResolver)
	if len(ns.addrs) == 0 && ns.srvName == "" {
		return nil, errors.New("dqlite: no node addresses and no discovery SRV configured")
	}

	drv, err := dqlitedriver.New(ns)
	if err != nil {
		return nil, errors.Wrap(err, "dqlite: driver")
	}
	connector, err := drv.OpenConnector(database)
	if err != nil {
		return nil, errors.Wrap(err, "dqlite: connector")
	}

	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, errors.Wrapf(err, "dqlite: ping failed for %v", ns.addrs)
	}
	return db, nil
}
